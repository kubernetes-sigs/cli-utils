// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// The solver package is responsible for constructing a
// taskqueue based on the set of resources that should be
// applied.
// This involves setting up the appropriate sequence of
// apply, wait and prune tasks so any dependencies between
// resources doesn't cause a later apply operation to
// fail.
// Currently this package assumes that the resources have
// already been sorted in the appropriate order. We might
// want to consider moving the sorting functionality into
// this package.
package solver

import (
	"fmt"
	"time"

	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/task"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type TaskQueueSolver struct {
	PruneOptions *prune.PruneOptions
	InfoHelper   info.InfoHelper
	Factory      util.Factory
	Mapper       meta.RESTMapper
	InvClient    inventory.InventoryClient
}

type TaskQueue struct {
	tasks []taskrunner.Task
}

func (tq *TaskQueue) ToChannel() chan taskrunner.Task {
	taskQueue := make(chan taskrunner.Task, len(tq.tasks))
	for _, t := range tq.tasks {
		taskQueue <- t
	}
	return taskQueue
}

func (tq *TaskQueue) ToActionGroups() []event.ActionGroup {
	var ags []event.ActionGroup

	for _, t := range tq.tasks {
		ags = append(ags, event.ActionGroup{
			Name:        t.Name(),
			Action:      t.Action(),
			Identifiers: t.Identifiers(),
		})
	}
	return ags
}

type Options struct {
	ServerSideOptions      common.ServerSideOptions
	ReconcileTimeout       time.Duration
	Prune                  bool
	DryRunStrategy         common.DryRunStrategy
	PrunePropagationPolicy metav1.DeletionPropagation
	PruneTimeout           time.Duration
	InventoryPolicy        inventory.InventoryPolicy
}

type resourceObjects interface {
	ObjsForApply() []*unstructured.Unstructured
	Inventory() inventory.InventoryInfo
	IdsForApply() []object.ObjMetadata
	IdsForPrune() []object.ObjMetadata
	IdsForPrevInv() []object.ObjMetadata
}

// BuildTaskQueue takes a set of resources in the form of info objects
// and constructs the task queue. The options parameter allows
// customization of how the task queue are built.
func (t *TaskQueueSolver) BuildTaskQueue(ro resourceObjects,
	o Options) *TaskQueue {
	var applyCounter int
	var pruneCounter int
	var waitCounter int

	var tasks []taskrunner.Task
	remainingInfos := ro.ObjsForApply()
	// Convert slice of previous inventory objects into a map.
	prevInvSlice := ro.IdsForPrevInv()
	prevInventory := make(map[object.ObjMetadata]bool, len(prevInvSlice))
	for _, prevInvObj := range prevInvSlice {
		prevInventory[prevInvObj] = true
	}

	// First task merge local applied objects to inventory.
	tasks = append(tasks, &task.InvAddTask{
		InvClient: t.InvClient,
		InvInfo:   ro.Inventory(),
		Objects:   ro.ObjsForApply(),
	})

	crdSplitRes, hasCRDs := splitAfterCRDs(remainingInfos)
	if hasCRDs {
		tasks = append(tasks, &task.ApplyTask{
			TaskName:          fmt.Sprintf("apply-%d", applyCounter),
			Objects:           append(crdSplitRes.before, crdSplitRes.crds...),
			CRDs:              crdSplitRes.crds,
			PrevInventory:     prevInventory,
			ServerSideOptions: o.ServerSideOptions,
			DryRunStrategy:    o.DryRunStrategy,
			InfoHelper:        t.InfoHelper,
			Factory:           t.Factory,
			Mapper:            t.Mapper,
			InventoryPolicy:   o.InventoryPolicy,
			InvInfo:           ro.Inventory(),
		})
		applyCounter += 1
		if !o.DryRunStrategy.ClientOrServerDryRun() {
			objs := object.UnstructuredsToObjMetas(crdSplitRes.crds)
			tasks = append(tasks, taskrunner.NewWaitTask(
				fmt.Sprintf("wait-%d", waitCounter),
				objs,
				taskrunner.AllCurrent,
				1*time.Minute,
				t.Mapper),
			)
			waitCounter += 1
		}
		remainingInfos = crdSplitRes.after
	}

	tasks = append(tasks,
		&task.ApplyTask{
			TaskName:          fmt.Sprintf("apply-%d", applyCounter),
			Objects:           remainingInfos,
			CRDs:              crdSplitRes.crds,
			PrevInventory:     prevInventory,
			ServerSideOptions: o.ServerSideOptions,
			DryRunStrategy:    o.DryRunStrategy,
			InfoHelper:        t.InfoHelper,
			Factory:           t.Factory,
			Mapper:            t.Mapper,
			InventoryPolicy:   o.InventoryPolicy,
			InvInfo:           ro.Inventory(),
		},
	)

	if !o.DryRunStrategy.ClientOrServerDryRun() && o.ReconcileTimeout != time.Duration(0) {
		tasks = append(tasks,
			taskrunner.NewWaitTask(
				fmt.Sprintf("wait-%d", waitCounter),
				object.UnstructuredsToObjMetas(remainingInfos),
				taskrunner.AllCurrent,
				o.ReconcileTimeout,
				t.Mapper),
		)
		waitCounter += 1
	}

	if o.Prune {
		tasks = append(tasks,
			&task.PruneTask{
				TaskName:          fmt.Sprintf("prune-%d", pruneCounter),
				Objects:           ro.ObjsForApply(),
				InventoryObject:   ro.Inventory(),
				PruneOptions:      t.PruneOptions,
				PropagationPolicy: o.PrunePropagationPolicy,
				DryRunStrategy:    o.DryRunStrategy,
				InventoryPolicy:   o.InventoryPolicy,
			},
		)

		if !o.DryRunStrategy.ClientOrServerDryRun() && o.PruneTimeout != time.Duration(0) {
			tasks = append(tasks,
				taskrunner.NewWaitTask(
					fmt.Sprintf("wait-%d", waitCounter),
					ro.IdsForPrune(),
					taskrunner.AllNotFound,
					o.PruneTimeout,
					t.Mapper),
			)
		}
	}

	// Final task is to set inventory with objects in cluster.
	tasks = append(tasks, &task.InvSetTask{
		InvClient: t.InvClient,
		InvInfo:   ro.Inventory(),
	})

	return &TaskQueue{
		tasks: tasks,
	}
}

type crdSplitResult struct {
	before []*unstructured.Unstructured
	after  []*unstructured.Unstructured
	crds   []*unstructured.Unstructured
}

// splitAfterCRDs takes a sorted slice of infos and splits it into
// three parts; resources before CRDs, the CRDs themselves, and finally
// all the resources after the CRDs.
// The function returns the three different sets of resources and
// a boolean that tells whether there were any CRDs in the set of
// resources.
func splitAfterCRDs(objs []*unstructured.Unstructured) (crdSplitResult, bool) {
	var before []*unstructured.Unstructured
	var after []*unstructured.Unstructured

	var crds []*unstructured.Unstructured
	for _, obj := range objs {
		if IsCRD(obj) {
			crds = append(crds, obj)
			continue
		}

		if len(crds) > 0 {
			after = append(after, obj)
		} else {
			before = append(before, obj)
		}
	}
	return crdSplitResult{
		before: before,
		after:  after,
		crds:   crds,
	}, len(crds) > 0
}

func IsCRD(info *unstructured.Unstructured) bool {
	gvk, found := toGVK(info)
	if !found {
		return false
	}
	if (gvk.Group == v1.SchemeGroupVersion.Group ||
		gvk.Group == v1beta1.SchemeGroupVersion.Group) &&
		gvk.Kind == "CustomResourceDefinition" {
		return true
	}
	return false
}

func toGVK(obj *unstructured.Unstructured) (schema.GroupVersionKind, bool) {
	if obj != nil {
		return obj.GroupVersionKind(), true
	}
	return schema.GroupVersionKind{}, false
}

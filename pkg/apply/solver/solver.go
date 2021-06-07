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
	"k8s.io/klog/v2"
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

const defaultWaitTimeout = 1 * time.Minute

type TaskQueueBuilder struct {
	PruneOptions *prune.PruneOptions
	InfoHelper   info.InfoHelper
	Factory      util.Factory
	Mapper       meta.RESTMapper
	InvClient    inventory.InventoryClient
	// The accumulated tasks and counter variables to name tasks.
	invAddCounter    int
	invSetCounter    int
	deleteInvCounter int
	applyCounter     int
	waitCounter      int
	pruneCounter     int
	tasks            []taskrunner.Task
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

// Build returns the queue of tasks that have been created.
func (t *TaskQueueBuilder) Build() *TaskQueue {
	return &TaskQueue{
		tasks: t.tasks,
	}
}

// AppendInvAddTask appends an inventory add task to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendInvAddTask(ro resourceObjects) *TaskQueueBuilder {
	klog.V(5).Infoln("adding inventory add task")
	t.tasks = append(t.tasks, &task.InvAddTask{
		TaskName:  fmt.Sprintf("inventory-add-%d", t.invAddCounter),
		InvClient: t.InvClient,
		InvInfo:   ro.Inventory(),
		Objects:   ro.ObjsForApply(),
	})
	t.invAddCounter += 1
	return t
}

// AppendInvAddTask appends an inventory set task to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendInvSetTask(ro resourceObjects) *TaskQueueBuilder {
	klog.V(5).Infoln("adding inventory set task")
	t.tasks = append(t.tasks, &task.InvSetTask{
		TaskName:  fmt.Sprintf("inventory-set-%d", t.invSetCounter),
		InvClient: t.InvClient,
		InvInfo:   ro.Inventory(),
	})
	t.invSetCounter += 1
	return t
}

// AppendInvAddTask appends to the task queue a task to delete the inventory object.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendDeleteInvTask(ro resourceObjects) *TaskQueueBuilder {
	klog.V(5).Infoln("adding delete inventory task")
	t.tasks = append(t.tasks, &task.DeleteInvTask{
		TaskName:  fmt.Sprintf("delete-inventory-%d", t.deleteInvCounter),
		InvClient: t.InvClient,
		InvInfo:   ro.Inventory(),
	})
	t.deleteInvCounter += 1
	return t
}

// AppendInvAddTask appends a task to the task queue to apply the passed objects
// to the cluster. Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendApplyTask(applyObjs []*unstructured.Unstructured,
	crdSplitRes crdSplitResult, ro resourceObjects, o Options) *TaskQueueBuilder {
	klog.V(5).Infoln("adding apply task")
	// Convert slice of previous inventory objects into a map.
	prevInvSlice := ro.IdsForPrevInv()
	prevInventory := make(map[object.ObjMetadata]bool, len(prevInvSlice))
	for _, prevInvObj := range prevInvSlice {
		prevInventory[prevInvObj] = true
	}
	t.tasks = append(t.tasks, &task.ApplyTask{
		TaskName:          fmt.Sprintf("apply-%d", t.applyCounter),
		Objects:           applyObjs,
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
	t.applyCounter += 1
	return t
}

// AppendInvAddTask appends a task to wait on the passed objects to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendWaitTask(waitIds []object.ObjMetadata) *TaskQueueBuilder {
	klog.V(5).Infoln("adding wait task")
	t.tasks = append(t.tasks, taskrunner.NewWaitTask(
		fmt.Sprintf("wait-%d", t.waitCounter),
		waitIds,
		taskrunner.AllCurrent,
		defaultWaitTimeout,
		t.Mapper),
	)
	t.waitCounter += 1
	return t
}

// AppendInvAddTask appends a task to delete objects from the cluster to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendPruneTask(ro resourceObjects, o Options) *TaskQueueBuilder {
	klog.V(5).Infoln("adding prune task")
	t.tasks = append(t.tasks,
		&task.PruneTask{
			TaskName:          fmt.Sprintf("prune-%d", t.pruneCounter),
			Objects:           ro.ObjsForApply(),
			InventoryObject:   ro.Inventory(),
			PruneOptions:      t.PruneOptions,
			PropagationPolicy: o.PrunePropagationPolicy,
			DryRunStrategy:    o.DryRunStrategy,
			InventoryPolicy:   o.InventoryPolicy,
		},
	)
	t.pruneCounter += 1
	return t
}

// AppendApplyWaitTasks adds apply and wait tasks to the task queue,
// depending on build variables (like dry-run) and resource types
// (like CRD's). Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendApplyWaitTasks(ro resourceObjects, o Options) *TaskQueueBuilder {
	applyObjs := ro.ObjsForApply()
	crdSplitRes, hasCRDs := splitAfterCRDs(applyObjs)
	if hasCRDs {
		t.AppendApplyTask(append(crdSplitRes.before, crdSplitRes.crds...), crdSplitRes, ro, o)
		if !o.DryRunStrategy.ClientOrServerDryRun() {
			waitIds := object.UnstructuredsToObjMetas(crdSplitRes.crds)
			t.AppendWaitTask(waitIds)
		}
		applyObjs = crdSplitRes.after
	}
	t.AppendApplyTask(applyObjs, crdSplitRes, ro, o)
	if !o.DryRunStrategy.ClientOrServerDryRun() && o.ReconcileTimeout != time.Duration(0) {
		waitIds := object.UnstructuredsToObjMetas(applyObjs)
		t.AppendWaitTask(waitIds)
	}
	return t
}

// AppendPruneWaitTasks adds prune and wait tasks to the task queue
// based on build variables (like dry-run). Returns a pointer to the
// Builder to chain function calls.
func (t *TaskQueueBuilder) AppendPruneWaitTasks(ro resourceObjects, o Options) *TaskQueueBuilder {
	if o.Prune {
		t.AppendPruneTask(ro, o)
		if !o.DryRunStrategy.ClientOrServerDryRun() && o.PruneTimeout != time.Duration(0) {
			t.AppendWaitTask(ro.IdsForPrune())
		}
	}
	return t
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

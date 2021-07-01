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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/task"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/graph"
)

const defaultWaitTimeout = 1 * time.Minute

type TaskQueueBuilder struct {
	PruneOptions *prune.PruneOptions
	InfoHelper   info.InfoHelper
	Factory      util.Factory
	Mapper       meta.RESTMapper
	InvClient    inventory.InventoryClient
	// True if we are destroying, which deletes the inventory object
	// as well (possibly) the inventory namespace.
	Destroy bool
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

// Build returns the queue of tasks that have been created.
func (t *TaskQueueBuilder) Build() *TaskQueue {
	return &TaskQueue{
		tasks: t.tasks,
	}
}

// AppendInvAddTask appends an inventory add task to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendInvAddTask(inv inventory.InventoryInfo, applyObjs []*unstructured.Unstructured) *TaskQueueBuilder {
	klog.V(2).Infoln("adding inventory add task")
	t.tasks = append(t.tasks, &task.InvAddTask{
		TaskName:  fmt.Sprintf("inventory-add-%d", t.invAddCounter),
		InvClient: t.InvClient,
		InvInfo:   inv,
		Objects:   applyObjs,
	})
	t.invAddCounter += 1
	return t
}

// AppendInvAddTask appends an inventory set task to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendInvSetTask(inv inventory.InventoryInfo) *TaskQueueBuilder {
	klog.V(2).Infoln("adding inventory set task")
	t.tasks = append(t.tasks, &task.InvSetTask{
		TaskName:  fmt.Sprintf("inventory-set-%d", t.invSetCounter),
		InvClient: t.InvClient,
		InvInfo:   inv,
	})
	t.invSetCounter += 1
	return t
}

// AppendInvAddTask appends to the task queue a task to delete the inventory object.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendDeleteInvTask(inv inventory.InventoryInfo) *TaskQueueBuilder {
	klog.V(2).Infoln("adding delete inventory task")
	t.tasks = append(t.tasks, &task.DeleteInvTask{
		TaskName:  fmt.Sprintf("delete-inventory-%d", t.deleteInvCounter),
		InvClient: t.InvClient,
		InvInfo:   inv,
	})
	t.deleteInvCounter += 1
	return t
}

// AppendInvAddTask appends a task to the task queue to apply the passed objects
// to the cluster. Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendApplyTask(inv inventory.InventoryInfo,
	applyObjs []*unstructured.Unstructured, o Options) *TaskQueueBuilder {
	klog.V(2).Infof("adding apply task (%d objects)", len(applyObjs))
	prevInvIds, _ := t.InvClient.GetClusterObjs(inv)
	prevInventory := make(map[object.ObjMetadata]bool, len(prevInvIds))
	for _, prevInvID := range prevInvIds {
		prevInventory[prevInvID] = true
	}
	t.tasks = append(t.tasks, &task.ApplyTask{
		TaskName:          fmt.Sprintf("apply-%d", t.applyCounter),
		Objects:           applyObjs,
		PrevInventory:     prevInventory,
		ServerSideOptions: o.ServerSideOptions,
		DryRunStrategy:    o.DryRunStrategy,
		InfoHelper:        t.InfoHelper,
		Factory:           t.Factory,
		Mapper:            t.Mapper,
		InventoryPolicy:   o.InventoryPolicy,
		InvInfo:           inv,
	})
	t.applyCounter += 1
	return t
}

// AppendInvAddTask appends a task to wait on the passed objects to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendWaitTask(waitIds []object.ObjMetadata, condition taskrunner.Condition,
	waitTimeout time.Duration) *TaskQueueBuilder {
	klog.V(2).Infoln("adding wait task")
	t.tasks = append(t.tasks, taskrunner.NewWaitTask(
		fmt.Sprintf("wait-%d", t.waitCounter),
		waitIds,
		condition,
		waitTimeout,
		t.Mapper),
	)
	t.waitCounter += 1
	return t
}

// AppendInvAddTask appends a task to delete objects from the cluster to the task queue.
// Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendPruneTask(pruneObjs []*unstructured.Unstructured,
	pruneFilters []filter.ValidationFilter, o Options) *TaskQueueBuilder {
	klog.V(2).Infof("adding prune task (%d objects)", len(pruneObjs))
	t.tasks = append(t.tasks,
		&task.PruneTask{
			TaskName:          fmt.Sprintf("prune-%d", t.pruneCounter),
			Objects:           pruneObjs,
			Filters:           pruneFilters,
			PruneOptions:      t.PruneOptions,
			PropagationPolicy: o.PrunePropagationPolicy,
			DryRunStrategy:    o.DryRunStrategy,
			Destroy:           t.Destroy,
		},
	)
	t.pruneCounter += 1
	return t
}

// AppendApplyWaitTasks adds apply and wait tasks to the task queue,
// depending on build variables (like dry-run) and resource types
// (like CRD's). Returns a pointer to the Builder to chain function calls.
func (t *TaskQueueBuilder) AppendApplyWaitTasks(inv inventory.InventoryInfo,
	applyObjs []*unstructured.Unstructured, o Options) *TaskQueueBuilder {
	// Use the "depends-on" annotation to create a graph, ands sort the
	// objects to apply into sets using a topological sort.
	applySets := graph.SortObjs(applyObjs)
	addWaitTask, waitTimeout := waitTaskTimeout(o.DryRunStrategy.ClientOrServerDryRun(),
		len(applySets), o.ReconcileTimeout)
	for _, applySet := range applySets {
		t.AppendApplyTask(inv, applySet, o)
		if addWaitTask {
			applyIds := object.UnstructuredsToObjMetasOrDie(applySet)
			t.AppendWaitTask(applyIds, taskrunner.AllCurrent, waitTimeout)
		}
	}
	return t
}

// AppendPruneWaitTasks adds prune and wait tasks to the task queue
// based on build variables (like dry-run). Returns a pointer to the
// Builder to chain function calls.
func (t *TaskQueueBuilder) AppendPruneWaitTasks(pruneObjs []*unstructured.Unstructured,
	pruneFilters []filter.ValidationFilter, o Options) *TaskQueueBuilder {
	if o.Prune {
		// Use the "depends-on" annotation to create a graph, ands sort the
		// objects to prune into sets using a (reverse) topological sort.
		pruneSets := graph.ReverseSortObjs(pruneObjs)
		addWaitTask, waitTimeout := waitTaskTimeout(o.DryRunStrategy.ClientOrServerDryRun(),
			len(pruneSets), o.ReconcileTimeout)
		for _, pruneSet := range pruneSets {
			t.AppendPruneTask(pruneSet, pruneFilters, o)
			if addWaitTask {
				pruneIds := object.UnstructuredsToObjMetasOrDie(pruneSet)
				t.AppendWaitTask(pruneIds, taskrunner.AllNotFound, waitTimeout)
			}
		}
	}
	return t
}

// waitTaskTimeout returns true if the wait task should be added to the task queue;
// false otherwise. If true, also returns the duration within wait task before timeout.
func waitTaskTimeout(dryRun bool, numObjSets int, reconcileTimeout time.Duration) (bool, time.Duration) {
	var zeroTimeout = time.Duration(0)
	if dryRun {
		return false, zeroTimeout
	}
	if reconcileTimeout != zeroTimeout {
		return true, reconcileTimeout
	}
	if numObjSets > 1 {
		return true, defaultWaitTimeout
	}
	return false, zeroTimeout
}

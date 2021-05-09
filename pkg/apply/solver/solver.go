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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/task"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/graph"
)

const defaultWaitTimeout = time.Minute

type TaskQueueSolver struct {
	PruneOptions *prune.PruneOptions
	InfoHelper   info.InfoHelper
	Factory      util.Factory
	Mapper       meta.RESTMapper
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
	o Options) chan taskrunner.Task {
	var tasks []taskrunner.Task
	// Convert slice of previous inventory objects into a map.
	prevInvSlice := ro.IdsForPrevInv()
	prevInventory := make(map[object.ObjMetadata]bool, len(prevInvSlice))
	for _, prevInvObj := range prevInvSlice {
		prevInventory[prevInvObj] = true
	}
	// Use the "depends-on" annotation to create a graph, ands sort the
	// objects to apply into sets using a topological sort.
	applySets := sortApplyObjs(ro.ObjsForApply())
	addWaitTask, waitTimeout := waitTaskTimeout(o.DryRunStrategy.ClientOrServerDryRun(),
		len(applySets), o.ReconcileTimeout)
	for i, applySet := range applySets {
		objs := object.UnstructuredsToObjMetas(applySet)
		klog.V(2).Infof("adding apply task: %s\n", objs)
		tasks = append(tasks, t.NewApplyTask(applySet, prevInventory, ro, o))
		// After last apply task, add "apply event completed" task.
		if i >= len(applySets)-1 {
			tasks = append(tasks, newApplyEventCompletedTask())
		}
		if addWaitTask {
			klog.V(2).Infof("adding apply wait task: %s\n", objs)
			tasks = append(tasks, taskrunner.NewWaitTask(objs, taskrunner.AllCurrent, waitTimeout))
			if resetRestMapper(applySet) {
				klog.V(2).Infoln("adding reset RESTMapper task")
				tasks = append(tasks, &task.ResetRESTMapperTask{Mapper: t.Mapper})
			}
		}
	}
	// After last wait task, add "status event completed" task.
	if addWaitTask {
		tasks = append(tasks, newStatusEventCompletedTask())
	}
	if o.Prune {
		// TODO(seans): reverse order the object to prune based on "depends-on" annotation.
		klog.V(2).Infof("adding prune task: %s\n", ro.IdsForPrune())
		tasks = append(tasks,
			t.NewPruneTask(ro, o),
			newPruneEventCompletedTask(),
		)

		if !o.DryRunStrategy.ClientOrServerDryRun() && o.PruneTimeout != time.Duration(0) {
			klog.V(2).Infof("adding prune wait task: %s\n", ro.IdsForPrune())
			tasks = append(tasks,
				taskrunner.NewWaitTask(ro.IdsForPrune(), taskrunner.AllNotFound, o.PruneTimeout),
				newStatusEventCompletedTask(),
			)
		}
	}
	return tasksToQueue(tasks)
}

// waitTaskTimeout returns true if the wait task should be added to the task queue;
// false otherwise. If true, also returns the duration within wait task before timeout.
// Add a wait task if:
//  1) non-dry-run
//  2) AND multiple apply sets
//  3) OR single apply set && reconcileTimeout is set
func waitTaskTimeout(dryRun bool, numApplySets int, reconcileTimeout time.Duration) (bool, time.Duration) {
	var zeroTimeout = time.Duration(0)
	if dryRun {
		return false, zeroTimeout
	}
	if reconcileTimeout != zeroTimeout {
		return true, reconcileTimeout
	}
	if numApplySets > 1 {
		return true, defaultWaitTimeout
	}
	return false, zeroTimeout
}

// resetRestMapper returns true if any of the passed objects
// are Kind == CustomResourceDefinition; false otherwise.
func resetRestMapper(objs []*unstructured.Unstructured) bool {
	for _, obj := range objs {
		if IsCRD(obj) {
			return true
		}
	}
	return false
}

func (t *TaskQueueSolver) NewApplyTask(objs []*unstructured.Unstructured,
	prevInventory map[object.ObjMetadata]bool,
	ro resourceObjects,
	o Options) taskrunner.Task {
	return &task.ApplyTask{
		Objects:           objs,
		CRDs:              []*unstructured.Unstructured{},
		PrevInventory:     prevInventory,
		ServerSideOptions: o.ServerSideOptions,
		DryRunStrategy:    o.DryRunStrategy,
		InfoHelper:        t.InfoHelper,
		Factory:           t.Factory,
		Mapper:            t.Mapper,
		InventoryPolicy:   o.InventoryPolicy,
		InvInfo:           ro.Inventory(),
	}
}

func (t *TaskQueueSolver) NewPruneTask(ro resourceObjects, o Options) taskrunner.Task {
	return &task.PruneTask{
		Objects:           ro.ObjsForApply(),
		InventoryObject:   ro.Inventory(),
		PruneOptions:      t.PruneOptions,
		PropagationPolicy: o.PrunePropagationPolicy,
		DryRunStrategy:    o.DryRunStrategy,
		InventoryPolicy:   o.InventoryPolicy,
	}
}

func newApplyEventCompletedTask() taskrunner.Task {
	return &task.SendEventTask{
		Event: event.Event{
			Type: event.ApplyType,
			ApplyEvent: event.ApplyEvent{
				Type: event.ApplyEventCompleted,
			},
		},
	}
}

func newStatusEventCompletedTask() taskrunner.Task {
	return &task.SendEventTask{
		Event: event.Event{
			Type: event.StatusType,
			StatusEvent: event.StatusEvent{
				Type: event.StatusEventCompleted,
			},
		},
	}
}

func newPruneEventCompletedTask() taskrunner.Task {
	return &task.SendEventTask{
		Event: event.Event{
			Type: event.PruneType,
			PruneEvent: event.PruneEvent{
				Type: event.PruneEventCompleted,
			},
		},
	}
}

func tasksToQueue(tasks []taskrunner.Task) chan taskrunner.Task {
	taskQueue := make(chan taskrunner.Task, len(tasks))
	for _, t := range tasks {
		taskQueue <- t
	}
	return taskQueue
}

// sortApplyObjs returns a slice of the sets of objects to apply (in order).
// Each of the objects in an apply set is applied together. The order of
// the returned applied sets is a topological ordering of the sets to apply.
// Returns an single empty apply set if there are no objects to apply (which
// causes a single empty apply to be created).
func sortApplyObjs(applyObjs []*unstructured.Unstructured) [][]*unstructured.Unstructured {
	klog.V(3).Infof("sorting %d apply objects\n", len(applyObjs))
	if len(applyObjs) == 0 {
		return [][]*unstructured.Unstructured{{}}
	}
	g := graph.New()
	objToUnstructured := map[object.ObjMetadata]*unstructured.Unstructured{}
	// Add directed edges to the graph for explicit "depends-on" annotations.
	for _, applyObj := range applyObjs {
		obj := object.UnstructuredToObjMeta(applyObj)
		klog.V(3).Infof("adding vertex: %s\n", obj)
		g.AddVertex(obj)
		deps := object.DependsOnObjs(applyObj)
		for _, dep := range deps {
			klog.V(3).Infof("adding edge from: %s, to: %s\n", obj, dep)
			g.AddEdge(obj, dep)
		}
		objToUnstructured[obj] = applyObj
	}
	// Add directed edges to the graph for objects which are applied
	// with their namespaces. A dependency edge is added from object
	// to the namespace it is applied to.
	addNamespaceEdges(g, applyObjs)
	// Add directed edges to the graph for custom resources applied
	// with their definitions (CRD's).
	addCRDEdges(g, applyObjs)
	// Run topological sort on the graph, and map the object
	// metadata back to the unstructured objects.
	objs := [][]*unstructured.Unstructured{}
	sortedObjs, err := g.Sort()
	if err != nil {
		return objs
	}
	for _, applySet := range sortedObjs {
		currentSet := []*unstructured.Unstructured{}
		for _, obj := range applySet {
			if u, found := objToUnstructured[obj]; found {
				currentSet = append(currentSet, u)
			}
		}
		objs = append(objs, currentSet)
	}
	klog.V(3).Infof("sorted into %d apply sets\n", len(objs))
	return objs
}

// addCRDEdges adds edges to the dependency graph from custom
// resources to their definitions to ensure the CRD's exist
// before applying the custom resources created with the definition.
func addCRDEdges(g *graph.Graph, objs []*unstructured.Unstructured) {
	crds := map[string]object.ObjMetadata{}
	// First create a map of all the CRD's.
	for _, u := range objs {
		if IsCRD(u) {
			groupKind, found := getCRDGroupKind(u)
			if found {
				obj := object.UnstructuredToObjMeta(u)
				crds[groupKind.String()] = obj
			}
		}
	}
	// Iterate through all resources to see if we are applying any
	// custom resources using and applied CRD's.
	for _, u := range objs {
		gvk := u.GroupVersionKind()
		groupKind := gvk.GroupKind()
		if to, found := crds[groupKind.String()]; found {
			from := object.UnstructuredToObjMeta(u)
			klog.V(3).Infof("adding edge from: custom resource %s, to CRD: %s\n", from, to)
			g.AddEdge(from, to)
		}
	}
}

func IsCRD(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	gvk := u.GroupVersionKind()
	return object.ExtensionsCRD == gvk.GroupKind()
}

func getCRDGroupKind(u *unstructured.Unstructured) (schema.GroupKind, bool) {
	emptyGroupKind := schema.GroupKind{Group: "", Kind: ""}
	if u == nil {
		return emptyGroupKind, false
	}
	group, found, err := unstructured.NestedString(u.Object, "spec", "group")
	if found && err == nil {
		kind, found, err := unstructured.NestedString(u.Object, "spec", "names", "kind")
		if found && err == nil {
			return schema.GroupKind{Group: group, Kind: kind}, true
		}
	}
	return emptyGroupKind, false
}

// addNamespaceEdges adds edges to the dependency graph from namespaced
// objects to the namespace objects. Ensures the namespaces exist
// before the resources in those namespaces are applied.
func addNamespaceEdges(g *graph.Graph, objs []*unstructured.Unstructured) {
	namespaces := map[string]object.ObjMetadata{}
	// First create a map of all the namespaces we're applying.
	for _, u := range objs {
		if isKindNamespace(u) {
			obj := object.UnstructuredToObjMeta(u)
			namespace := u.GetName()
			namespaces[namespace] = obj
		}
	}
	// Next, if the namespace of a namespaced object is being applied,
	// then create an edge from the namespaced object to its namespace.
	for _, u := range objs {
		if isNamespaced(u) {
			objNamespace := u.GetNamespace()
			if namespace, found := namespaces[objNamespace]; found {
				obj := object.UnstructuredToObjMeta(u)
				klog.V(3).Infof("adding edge from: %s to namespace: %s\n", obj, namespace)
				g.AddEdge(obj, namespace)
			}
		}
	}
}

func isKindNamespace(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	gvk := u.GroupVersionKind()
	return object.CoreNamespace == gvk.GroupKind()
}

func isNamespaced(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	return u.GetNamespace() != ""
}

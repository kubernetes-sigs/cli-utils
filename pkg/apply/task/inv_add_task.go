// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// InvAddTask encapsulates structures necessary to add/merge inventory
// into the cluster. The InvAddTask should add/merge inventory references
// before the actual object is applied.
type InvAddTask struct {
	TaskName  string
	InvClient inventory.InventoryClient
	InvInfo   inventory.InventoryInfo
	Objects   []*unstructured.Unstructured
}

func (i *InvAddTask) Name() string {
	return i.TaskName
}

func (i *InvAddTask) Action() event.ResourceAction {
	return event.InventoryAction
}

func (i *InvAddTask) Identifiers() []object.ObjMetadata {
	return object.UnstructuredsToObjMetas(i.Objects)
}

// Start updates the inventory by merging the locally applied objects
// into the current inventory.
func (i *InvAddTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		if err := inventory.ValidateNoInventory(i.Objects); err != nil {
			taskContext.TaskChannel() <- taskrunner.TaskResult{Err: err}
			return
		}
		// Ensures the namespace exists before applying the inventory object into it.
		if invNamespace := inventoryNamespaceInSet(i.InvInfo, i.Objects); invNamespace != nil {
			klog.V(4).Infof("applying inventory namespace %s", invNamespace.GetName())
			if err := i.InvClient.ApplyInventoryNamespace(invNamespace); err != nil {
				taskContext.TaskChannel() <- taskrunner.TaskResult{Err: err}
				return
			}
		}
		klog.V(4).Infof("merging %d local objects into inventory", len(i.Objects))
		currentObjs := object.UnstructuredsToObjMetas(i.Objects)
		_, err := i.InvClient.Merge(i.InvInfo, currentObjs)
		taskContext.TaskChannel() <- taskrunner.TaskResult{Err: err}
	}()
}

// ClearTimeout is not supported by the InvAddTask.
func (i *InvAddTask) ClearTimeout() {}

// inventoryNamespaceInSet returns the the namespace the passed inventory
// object will be applied to, or nil if this namespace object does not exist
// in the passed slice "infos" or the inventory object is cluster-scoped.
func inventoryNamespaceInSet(inv inventory.InventoryInfo, objs []*unstructured.Unstructured) *unstructured.Unstructured {
	if inv == nil {
		return nil
	}
	invNamespace := inv.Namespace()

	for _, obj := range objs {
		acc, _ := meta.Accessor(obj)
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk == object.CoreV1Namespace && acc.GetName() == invNamespace {
			inventory.AddInventoryIDAnnotation(obj, inv)
			return obj
		}
	}
	return nil
}

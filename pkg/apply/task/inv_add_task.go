// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/inventory2"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var (
	namespaceGVKv1 = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
)

// InvAddTask encapsulates structures necessary to add/merge inventory
// into the cluster. The InvAddTask should add/merge inventory references
// before the actual object is applied.
type InvAddTask struct {
	TaskName      string
	InvClient     inventory2.Client
	DynamicClient dynamic.Interface
	Mapper        meta.RESTMapper
	InvInfo       inventory.Info
	Objects       object.UnstructuredSet
	DryRun        common.DryRunStrategy
	StatusPolicy  inventory.StatusPolicy
}

func (i *InvAddTask) Name() string {
	return i.TaskName
}

func (i *InvAddTask) Action() event.ResourceAction {
	return event.InventoryAction
}

func (i *InvAddTask) Identifiers() object.ObjMetadataSet {
	return object.UnstructuredSetToObjMetadataSet(i.Objects)
}

// Start updates the inventory by merging the locally applied objects
// into the current inventory.
func (i *InvAddTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		klog.V(2).Infof("inventory add task starting (name: %q)", i.Name())
		if err := inventory.ValidateNoInventory(i.Objects); err != nil {
			i.sendTaskResult(taskContext, err)
			return
		}
		// If the inventory is namespaced, ensure the namespace exists
		if i.InvInfo.Namespace() != "" {
			if invNamespace := inventoryNamespaceInSet(i.InvInfo, i.Objects); invNamespace != nil {
				if err := i.createNamespace(context.TODO(), invNamespace, i.DryRun); err != nil {
					err = fmt.Errorf("failed to create inventory namespace: %w", err)
					i.sendTaskResult(taskContext, err)
					return
				}
			}
		}
		klog.V(4).Infof("merging %d local objects into inventory", len(i.Objects))
		currentObjs := object.UnstructuredSetToObjMetadataSet(i.Objects)
		err := i.extendInventory(currentObjs)
		i.sendTaskResult(taskContext, err)
	}()
}

// extendInventory adds the specified objects to the inventory, if not already
// present.
func (i *InvAddTask) extendInventory(objs object.ObjMetadataSet) error {
	if len(objs) == 0 {
		return nil
	}
	id := inventory2.ID{
		Name:      i.InvInfo.Name(),
		Namespace: i.InvInfo.Namespace(),
	}
	inv, err := i.InvClient.Get(context.TODO(), id)
	if err != nil {
		return fmt.Errorf("getting inventory: %w")
	}

	oldObjs := inventory.ObjMetadataSetFromObjectReferenceList(inv.Spec.Objects)
	newObjs := oldObjs.Union(objs)
	inv.Spec.Objects = inventory.ObjectReferenceListFromObjMetadataSet(newObjs)

	if err = i.InvClient.Update(context.TODO(), inv, i.updateOptionList()...); err != nil {
		return fmt.Errorf("updating inventory: %w")
	}
	return nil
}

func (i *InvAddTask) updateOptionList() []inventory2.UpdateOption {
	return []inventory2.UpdateOption{
		inventory2.WithDryRun(i.DryRun),
		inventory2.WithStatus(i.StatusPolicy),
	}
}

// Cancel is not supported by the InvAddTask.
func (i *InvAddTask) Cancel(_ *taskrunner.TaskContext) {}

// StatusUpdate is not supported by the InvAddTask.
func (i *InvAddTask) StatusUpdate(_ *taskrunner.TaskContext, _ object.ObjMetadata) {}

// inventoryNamespaceInSet returns the the namespace the passed inventory
// object will be applied to, or nil if this namespace object does not exist
// in the passed slice "infos" or the inventory object is cluster-scoped.
func inventoryNamespaceInSet(inv inventory.Info, objs object.UnstructuredSet) *unstructured.Unstructured {
	if inv == nil {
		return nil
	}
	invNamespace := inv.Namespace()

	for _, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk == namespaceGVKv1 && obj.GetName() == invNamespace {
			inventory.AddInventoryIDAnnotation(obj, inv)
			return obj
		}
	}
	return nil
}

// createNamespace creates the specified namespace object
func (i *InvAddTask) createNamespace(ctx context.Context, obj *unstructured.Unstructured, dryRun common.DryRunStrategy) error {
	if dryRun.ClientOrServerDryRun() {
		klog.V(4).Infof("skipped applying inventory namespace (dry-run): %s", obj.GetName())
		return nil
	}
	klog.V(4).Infof("applying inventory namespace: %s", obj.GetName())

	nsObj := obj.DeepCopy()
	object.StripKyamlAnnotations(nsObj)
	if err := util.CreateApplyAnnotation(nsObj, unstructured.UnstructuredJSONScheme); err != nil {
		return err
	}

	mapping, err := i.getMapping(obj)
	if err != nil {
		return err
	}

	_, err = i.DynamicClient.Resource(mapping.Resource).Create(ctx, nsObj, metav1.CreateOptions{})
	return err
}

// getMapping returns the RESTMapping for the provided resource.
func (i *InvAddTask) getMapping(obj *unstructured.Unstructured) (*meta.RESTMapping, error) {
	gvk := obj.GroupVersionKind()
	return i.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
}

func (i *InvAddTask) sendTaskResult(taskContext *taskrunner.TaskContext, err error) {
	klog.V(2).Infof("inventory add task completing (name: %q)", i.Name())
	taskContext.TaskChannel() <- taskrunner.TaskResult{
		Err: err,
	}
}

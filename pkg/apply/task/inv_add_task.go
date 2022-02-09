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
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	namespaceGVKv1 = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
)

// InvAddTask encapsulates structures necessary to add/merge inventory
// into the cluster. The InvAddTask should add/merge inventory references
// before the actual object is applied.
type InvAddTask struct {
	TaskName      string
	InvClient     inventory.Client
	DynamicClient dynamic.Interface
	Mapper        meta.RESTMapper
	ApplyObjects  object.UnstructuredSet
	PruneObjects  object.UnstructuredSet
	DryRun        common.DryRunStrategy
}

func (i *InvAddTask) Name() string {
	return i.TaskName
}

func (i *InvAddTask) Action() event.ResourceAction {
	return event.InventoryAction
}

func (i *InvAddTask) Identifiers() object.ObjMetadataSet {
	return object.UnstructuredSetToObjMetadataSet(i.ApplyObjects).Union(
		object.UnstructuredSetToObjMetadataSet(i.PruneObjects))
}

// Start updates the inventory by merging the locally applied objects
// into the current inventory.
func (i *InvAddTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		klog.V(2).Infof("inventory add task starting (name: %q)", i.Name())
		// TODO: pipe Context through TaskContext
		ctx := context.TODO()

		// TODO: move to the Validator
		if err := inventory.ValidateNoInventory(i.ApplyObjects); err != nil {
			i.sendTaskResult(taskContext, err)
			return
		}
		// TODO: validate the PruneObjects don't contain the inventory
		// TODO: validate the PruneObjects don't contain the inventory namespace

		// Read inventory info from context
		inv := taskContext.InventoryManager().Inventory()

		// If the inventory is namespaced, ensure the namespace exists
		if inv.GetNamespace() != "" {
			if invNamespace := inventoryNamespaceInSet(inv, i.ApplyObjects); invNamespace != nil {
				if err := i.createNamespace(ctx, invNamespace, i.DryRun); err != nil {
					err = fmt.Errorf("failed to create inventory namespace: %w", err)
					i.sendTaskResult(taskContext, err)
					return
				}
			}
		}

		applyIds := object.UnstructuredSetToObjMetadataSet(i.ApplyObjects)
		deleteIds := object.UnstructuredSetToObjMetadataSet(i.PruneObjects)

		if overlap := applyIds.Intersection(deleteIds); len(overlap) > 0 {
			err := fmt.Errorf("apply set and delete set share objects: %v", overlap)
			i.sendTaskResult(taskContext, err)
			return
		}

		// Set the inventory objects
		inv.Spec.Objects = inventory.ObjectReferencesFromObjMetadataSet(
			applyIds.Union(deleteIds))

		// Set the inventory object status
		objStatusSet := make([]actuation.ObjectStatus, 0, len(applyIds)+len(deleteIds))
		for _, id := range applyIds {
			objStatusSet = append(objStatusSet, actuation.ObjectStatus{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(id),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationPending,
				Reconcile:       actuation.ReconcilePending,
			})
		}
		for _, id := range deleteIds {
			objStatusSet = append(objStatusSet, actuation.ObjectStatus{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(id),
				Strategy:        actuation.ActuationStrategyDelete,
				Actuation:       actuation.ActuationPending,
				Reconcile:       actuation.ReconcilePending,
			})
		}
		inv.Status.Objects = objStatusSet

		// Update the inventory
		err := i.InvClient.Store(inv, i.DryRun)
		if err != nil {
			err = fmt.Errorf("failed to update inventory: %w", err)
			i.sendTaskResult(taskContext, err)
			return
		}

		i.sendTaskResult(taskContext, nil)
	}()
}

// Cancel is not supported by the InvAddTask.
func (i *InvAddTask) Cancel(_ *taskrunner.TaskContext) {}

// StatusUpdate is not supported by the InvAddTask.
func (i *InvAddTask) StatusUpdate(_ *taskrunner.TaskContext, _ object.ObjMetadata) {}

// inventoryNamespaceInSet returns the the namespace the passed inventory
// object will be applied to, or nil if this namespace object does not exist
// in the passed slice "infos" or the inventory object is cluster-scoped.
func inventoryNamespaceInSet(inv client.Object, objs object.UnstructuredSet) *unstructured.Unstructured {
	invID := inventory.InventoryLabel(inv)
	invNamespace := inv.GetNamespace()
	for _, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk == namespaceGVKv1 && obj.GetName() == invNamespace {
			inventory.SetOwningInventoryAnnotation(obj, invID)
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

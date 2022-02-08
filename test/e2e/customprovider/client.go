// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package customprovider

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var InventoryGVK = schema.GroupVersionKind{
	Group:   "cli-utils.example.io",
	Version: "v1alpha1",
	Kind:    "Inventory",
}

type CustomInventoryClient struct {
	DynamicClient dynamic.Interface
	Mapper        meta.RESTMapper
}

var _ inventory.Client = &CustomInventoryClient{}

func (cic *CustomInventoryClient) GroupVersionKind() schema.GroupVersionKind {
	return InventoryGVK
}

func (cic *CustomInventoryClient) Load(invInfo inventory.InventoryInfo) (*actuation.Inventory, error) {
	klog.V(4).Infof("loading inventory: %v",
		inventory.NewObjectReferenceStringer(invInfo.ObjectReference))

	gk := inventory.GroupKindFromObjectReference(invInfo.ObjectReference)
	if gk != cic.GroupVersionKind().GroupKind() {
		return nil, fmt.Errorf("GroupKind not supported by CustomInventoryClient: %+v", gk)
	}
	id := inventory.ObjMetadataFromObjectReference(invInfo.ObjectReference)

	mapping, err := cic.Mapper.RESTMapping(id.GroupKind)
	if err != nil {
		return nil, err
	}

	obj, err := cic.getObject(context.TODO(), id, mapping)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	klog.V(7).Infof("Existing inventory:\n%s", object.YamlStringer{O: obj})

	if invInfo.ID != inventory.InventoryLabel(obj) {
		return nil, fmt.Errorf("inventory-id of inventory object in cluster doesn't match provided id %q", invInfo.ID)
	}

	var objs object.ObjMetadataSet
	s, found, err := unstructured.NestedSlice(obj.Object, "spec", "inventory")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	for _, item := range s {
		m := item.(map[string]interface{})
		namespace, _, _ := unstructured.NestedString(m, "namespace")
		name, _, _ := unstructured.NestedString(m, "name")
		group, _, _ := unstructured.NestedString(m, "group")
		kind, _, _ := unstructured.NestedString(m, "kind")
		id := object.ObjMetadata{
			Namespace: namespace,
			Name:      name,
			GroupKind: schema.GroupKind{
				Group: group,
				Kind:  kind,
			},
		}
		objs = append(objs, id)
	}

	inv := &actuation.Inventory{}
	inv.SetGroupVersionKind(cic.GroupVersionKind())
	inv.SetName(invInfo.Name)
	inv.SetNamespace(invInfo.Namespace)
	inventory.SetInventoryLabel(inv, invInfo.ID)
	// TODO: deep copy all other metadata and status?
	inv.Spec.Objects = inventory.ObjectReferencesFromObjMetadataSet(objs)

	return inv, nil
}

func (cic *CustomInventoryClient) Store(inv *actuation.Inventory) error {
	if inv == nil {
		return errors.New("inventory must not be nil")
	}
	invInfo := inventory.InventoryInfoFromObject(inv)
	klog.V(4).Infof("updating inventory: %v",
		inventory.NewObjectReferenceStringer(invInfo.ObjectReference))

	gvk := inv.GroupVersionKind()
	if gvk != cic.GroupVersionKind() {
		return fmt.Errorf("GroupVersionKind not supported by CustomInventoryClient: %+v", gvk)
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(inv.GroupVersionKind())
	obj.SetName(inv.GetName())
	obj.SetNamespace(inv.GetNamespace())
	inventory.SetInventoryLabel(obj, inventory.InventoryLabel(inv))

	var objs []interface{}
	for _, ref := range inv.Spec.Objects {
		objs = append(objs, map[string]interface{}{
			"group":     ref.Group,
			"kind":      ref.Kind,
			"namespace": ref.Namespace,
			"name":      ref.Name,
		})
	}
	if len(objs) > 0 {
		err := unstructured.SetNestedSlice(obj.Object, objs, "spec", "inventory")
		if err != nil {
			return err
		}
	}

	mapping, err := cic.Mapper.RESTMapping(gvk.GroupKind())
	if err != nil {
		return err
	}

	id := inventory.ObjMetadataFromObjectReference(invInfo.ObjectReference)
	// TODO: use kubectl code to get SSA and CSA impl
	oldObj, err := cic.getObject(context.TODO(), id, mapping)
	if err != nil {
		return err
	}
	var out *unstructured.Unstructured
	if oldObj != nil {
		klog.V(7).Infof("Existing inventory:\n%s", object.YamlStringer{O: oldObj})
		// copy metadata from existing object
		obj.SetUID(oldObj.GetUID())
		obj.SetResourceVersion(oldObj.GetResourceVersion())
		out, err = cic.updateObject(context.TODO(), obj, mapping)
		if err != nil {
			return err
		}
	} else {
		out, err = cic.createObject(context.TODO(), obj, mapping)
		if err != nil {
			return err
		}
	}
	klog.V(7).Infof("Updated inventory:\n%s", object.YamlStringer{O: out})
	return nil
}

func (cic *CustomInventoryClient) Delete(invInfo inventory.InventoryInfo) error {
	klog.V(4).Infof("deleting inventory: %v",
		inventory.NewObjectReferenceStringer(invInfo.ObjectReference))

	gk := inventory.GroupKindFromObjectReference(invInfo.ObjectReference)
	if gk != cic.GroupVersionKind().GroupKind() {
		return fmt.Errorf("GroupKind not supported by CustomInventoryClient: %+v", gk)
	}

	mapping, err := cic.Mapper.RESTMapping(gk)
	if err != nil {
		return err
	}

	id := inventory.ObjMetadataFromObjectReference(invInfo.ObjectReference)
	err = cic.deleteObject(context.TODO(), id, mapping)
	if err != nil {
		return err
	}
	return nil
}

func (cic *CustomInventoryClient) getObject(ctx context.Context, id object.ObjMetadata, mapping *meta.RESTMapping) (*unstructured.Unstructured, error) {
	klog.V(4).Infof("getting object from cluster: %v", id)
	obj, err := cic.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Get(ctx, id.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get object %q: %w", id, err)
	}
	return obj, nil
}

func (cic *CustomInventoryClient) createObject(ctx context.Context, obj *unstructured.Unstructured, mapping *meta.RESTMapping) (*unstructured.Unstructured, error) {
	id := object.UnstructuredToObjMetadata(obj)
	klog.V(4).Infof("updating object in cluster: %v", id)
	out, err := cic.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return out, fmt.Errorf("failed to create object %q: %w", id, err)
	}
	return out, nil
}

func (cic *CustomInventoryClient) updateObject(ctx context.Context, obj *unstructured.Unstructured, mapping *meta.RESTMapping) (*unstructured.Unstructured, error) {
	id := object.UnstructuredToObjMetadata(obj)
	klog.V(4).Infof("updating object in cluster: %v", id)
	out, err := cic.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return out, fmt.Errorf("failed to update object %q: %w", id, err)
	}
	return out, nil
}

func (cic *CustomInventoryClient) deleteObject(ctx context.Context, id object.ObjMetadata, mapping *meta.RESTMapping) error {
	klog.V(4).Infof("deleting object in cluster: %v", id)
	err := cic.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Delete(ctx, id.Name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete object %q: %w", id, err)
	}
	return err
}

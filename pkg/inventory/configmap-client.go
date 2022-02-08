// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Introduces the InventoryConfigMap struct which implements
// the Inventory interface. The InventoryConfigMap wraps a
// ConfigMap resource which stores the set of inventory
// (object metadata).

package inventory

import (
	"context"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// WrapInventoryObj takes a passed ConfigMap (as a resource.Info),
// wraps it with the InventoryConfigMap and upcasts the wrapper as
// an the Inventory interface.
// func WrapInventoryObj(inv *unstructured.Unstructured) Client {
// 	return &InventoryConfigMap{inv: inv}
// }

// InventoryToConfigMap converts an Inventory into an unstructured ConfigMap.
func InventoryToConfigMap(inv *actuation.Inventory) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(inv.GroupVersionKind())
	obj.SetName(inv.GetName())
	obj.SetNamespace(inv.GetNamespace())
	SetInventoryLabel(obj, InventoryLabel(inv))
	// TODO: deep copy all other metadata and status?
	objs := ObjMetadataSetFromObjectReferences(inv.Spec.Objects)
	err := unstructured.SetNestedStringMap(obj.Object, objs.ToStringMap(), "data")
	if err != nil {
		return obj, fmt.Errorf("failed to update ConfigMap data: %w", err)
	}
	return obj, nil
}

func NewConfigMapClient(DynamicClient dynamic.Interface, Mapper meta.RESTMapper) *ConfigMapClient {
	return &ConfigMapClient{
		DynamicClient: DynamicClient,
		Mapper:        Mapper,
	}
}

// ConfigMapClient wraps a ConfigMap resource and implements
// the Inventory interface. This wrapper loads and stores the
// object metadata (inventory) to and from the wrapped ConfigMap.
type ConfigMapClient struct {
	DynamicClient dynamic.Interface
	Mapper        meta.RESTMapper
}

var _ Client = &ConfigMapClient{}

func (cmc *ConfigMapClient) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}
}

// Load the inventory from a ConfigMap.
func (cmc *ConfigMapClient) Load(invInfo InventoryInfo) (*actuation.Inventory, error) {
	gk := GroupKindFromObjectReference(invInfo.ObjectReference)
	if gk != cmc.GroupVersionKind().GroupKind() {
		return nil, fmt.Errorf("GroupKind not supported by InventoryConfigMap: %+v", gk)
	}

	mapping, err := cmc.Mapper.RESTMapping(gk)
	if err != nil {
		return nil, err
	}

	id := ObjMetadataFromObjectReference(invInfo.ObjectReference)
	obj, err := cmc.getObject(context.TODO(), id, mapping)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	klog.V(7).Infof("Existing inventory:\n%s", object.YamlStringer{O: obj})

	if invInfo.ID != InventoryLabel(obj) {
		return nil, fmt.Errorf("inventory-id of inventory object in cluster doesn't match provided id %q", invInfo.ID)
	}

	objMap, exists, err := unstructured.NestedStringMap(obj.Object, "data")
	if err != nil {
		return nil, fmt.Errorf("failed to read ConfigMap data: %w", err)
	}
	var objs object.ObjMetadataSet
	if exists {
		objs, err = object.FromStringMap(objMap)
		if err != nil {
			return nil, err
		}
	}

	inv := &actuation.Inventory{}
	inv.SetGroupVersionKind(cmc.GroupVersionKind())
	inv.SetName(invInfo.Name)
	inv.SetNamespace(invInfo.Namespace)
	SetInventoryLabel(inv, invInfo.ID)
	// TODO: deep copy all other metadata and status?
	inv.Spec.Objects = ObjectReferencesFromObjMetadataSet(objs)
	sort.Sort(AlphanumericObjectReferences(inv.Spec.Objects))
	return inv, nil
}

// Store the inventory as a ConfigMap.
func (cmc *ConfigMapClient) Store(inv *actuation.Inventory) error {
	gvk := inv.GroupVersionKind()
	if gvk != cmc.GroupVersionKind() {
		return fmt.Errorf("GroupVersionKind not supported by InventoryConfigMap: %+v", gvk)
	}

	obj, err := InventoryToConfigMap(inv)
	if err != nil {
		return err
	}

	mapping, err := cmc.Mapper.RESTMapping(gvk.GroupKind())
	if err != nil {
		return err
	}

	invInfo := InventoryInfoFromObject(inv)
	id := ObjMetadataFromObjectReference(invInfo.ObjectReference)
	// TODO: use kubectl code to get SSA and CSA impl
	oldObj, err := cmc.getObject(context.TODO(), id, mapping)
	if err != nil {
		return err
	}
	var out *unstructured.Unstructured
	if oldObj != nil {
		klog.V(7).Infof("Existing inventory:\n%s", object.YamlStringer{O: oldObj})
		// copy metadata from existing object
		obj.SetUID(oldObj.GetUID())
		obj.SetResourceVersion(oldObj.GetResourceVersion())
		out, err = cmc.updateObject(context.TODO(), obj, mapping)
		if err != nil {
			return err
		}
	} else {
		out, err = cmc.createObject(context.TODO(), obj, mapping)
		if err != nil {
			return err
		}
	}
	klog.V(7).Infof("Updated inventory:\n%s", object.YamlStringer{O: out})
	return nil
}

func (cmc *ConfigMapClient) Delete(invInfo InventoryInfo) error {
	gk := GroupKindFromObjectReference(invInfo.ObjectReference)
	if gk != cmc.GroupVersionKind().GroupKind() {
		return fmt.Errorf("GroupKind not supported by InventoryConfigMap: %+v", gk)
	}

	mapping, err := cmc.Mapper.RESTMapping(gk)
	if err != nil {
		return err
	}

	id := ObjMetadataFromObjectReference(invInfo.ObjectReference)
	err = cmc.deleteObject(context.TODO(), id, mapping)
	if err != nil {
		return err
	}
	return nil
}

func (cmc *ConfigMapClient) getObject(ctx context.Context, id object.ObjMetadata, mapping *meta.RESTMapping) (*unstructured.Unstructured, error) {
	klog.V(4).Infof("getting object from cluster: %v", id)
	obj, err := cmc.DynamicClient.Resource(mapping.Resource).
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

func (cmc *ConfigMapClient) createObject(ctx context.Context, obj *unstructured.Unstructured, mapping *meta.RESTMapping) (*unstructured.Unstructured, error) {
	id := object.UnstructuredToObjMetadata(obj)
	klog.V(4).Infof("updating object in cluster: %v", id)
	out, err := cmc.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return out, fmt.Errorf("failed to create object %q: %w", id, err)
	}
	return out, nil
}

func (cmc *ConfigMapClient) updateObject(ctx context.Context, obj *unstructured.Unstructured, mapping *meta.RESTMapping) (*unstructured.Unstructured, error) {
	id := object.UnstructuredToObjMetadata(obj)
	klog.V(4).Infof("updating object in cluster: %v", id)
	out, err := cmc.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return out, fmt.Errorf("failed to update object %q: %w", id, err)
	}
	return out, nil
}

func (cmc *ConfigMapClient) deleteObject(ctx context.Context, id object.ObjMetadata, mapping *meta.RESTMapping) error {
	klog.V(4).Infof("deleting object in cluster: %v", id)
	err := cmc.DynamicClient.Resource(mapping.Resource).
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

type AlphanumericObjectReferences []actuation.ObjectReference

var _ sort.Interface = AlphanumericObjectReferences{}

func (a AlphanumericObjectReferences) Len() int      { return len(a) }
func (a AlphanumericObjectReferences) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a AlphanumericObjectReferences) Less(i, j int) bool {
	if a[i].Group != a[j].Group {
		return a[i].Group < a[j].Group
	}
	if a[i].Kind != a[j].Kind {
		return a[i].Kind < a[j].Kind
	}
	if a[i].Namespace != a[j].Namespace {
		return a[i].Namespace < a[j].Namespace
	}
	return a[i].Name < a[j].Name
}

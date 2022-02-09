// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package customprovider

import (
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var InventoryGVK = schema.GroupVersionKind{
	Group:   "cli-utils.example.io",
	Version: "v1alpha1",
	Kind:    "Inventory",
}

type CustomConverter struct {
}

var _ inventory.Converter = CustomConverter{}

// GroupVersionKind returns the GVK supported by this Converter
func (cc CustomConverter) GroupVersionKind() schema.GroupVersionKind {
	return InventoryGVK
}

// To converts from an Unstructured of the supported GVK to an Inventory
func (cc CustomConverter) To(obj *unstructured.Unstructured) (*actuation.Inventory, error) {
	inv := &actuation.Inventory{}
	// Copy TypeMeta
	inv.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	// Copy ObjectMeta
	object.DeepCopyObjectMetaInto(obj, inv)

	// Convert in.spec.inventory to out.spec.objects
	s, found, err := unstructured.NestedSlice(obj.UnstructuredContent(), "spec", "inventory")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	if len(s) > 0 {
		var ids object.ObjMetadataSet
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
			ids = append(ids, id)
		}
		inv.Spec.Objects = inventory.ObjectReferencesFromObjMetadataSet(ids)

		// Sort objects to reduce chun on update
		sort.Sort(inventory.AlphanumericObjectReferences(inv.Spec.Objects))
	}

	// TODO: copy status (not yet serialized)

	return inv, nil
}

// From converts from an Inventory to an Unstructured of the supported GVK
func (cc CustomConverter) From(inv *actuation.Inventory) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	// Copy TypeMeta
	obj.SetGroupVersionKind(inv.GetObjectKind().GroupVersionKind())
	// Copy ObjectMeta
	object.DeepCopyObjectMetaInto(inv, obj)

	// Convert in.spec.objects to out.spec.inventory
	if len(inv.Spec.Objects) > 0 {
		var objs []interface{}
		for _, ref := range inv.Spec.Objects {
			objs = append(objs, map[string]interface{}{
				"group":     ref.Group,
				"kind":      ref.Kind,
				"namespace": ref.Namespace,
				"name":      ref.Name,
			})
		}
		err := unstructured.SetNestedSlice(obj.Object, objs, "spec", "inventory")
		if err != nil {
			return nil, err
		}
	}

	// TODO: copy status (not yet serialized)

	return obj, nil
}

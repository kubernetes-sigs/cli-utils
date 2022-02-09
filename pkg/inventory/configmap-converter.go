// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type ConfigMapConverter struct {
}

var _ Converter = ConfigMapConverter{}

// GroupVersionKind returns the GVK supported by this Converter
func (cmc ConfigMapConverter) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}
}

// To converts from an Unstructured of the supported GVK to an Inventory
func (cmc ConfigMapConverter) To(obj *unstructured.Unstructured) (*actuation.Inventory, error) {
	inv := &actuation.Inventory{}
	// Copy TypeMeta
	inv.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	// Copy ObjectMeta
	object.DeepCopyObjectMetaInto(obj, inv)

	// Convert in.Data to out.Spec.Objects
	data, _, err := unstructured.NestedStringMap(obj.UnstructuredContent(), "data")
	if err != nil {
		return nil, fmt.Errorf("failed to read ConfigMap data: %w", err)
	}
	if len(data) > 0 {
		objs, err := object.FromStringMap(data)
		if err != nil {
			return nil, err
		}
		inv.Spec.Objects = ObjectReferencesFromObjMetadataSet(objs)

		// Sort objects to reduce chun on update
		sort.Sort(AlphanumericObjectReferences(inv.Spec.Objects))
	}

	// TODO: copy status (not yet serialized)

	return inv, nil
}

// From converts from an Inventory to an Unstructured of the supported GVK
func (cmc ConfigMapConverter) From(inv *actuation.Inventory) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	// Copy TypeMeta
	obj.SetGroupVersionKind(inv.GetObjectKind().GroupVersionKind())
	// Copy ObjectMeta
	object.DeepCopyObjectMetaInto(inv, obj)

	// Convert in.Spec.Objects to out.Data
	if len(inv.Spec.Objects) > 0 {
		ids := ObjMetadataSetFromObjectReferences(inv.Spec.Objects)
		err := unstructured.SetNestedStringMap(obj.Object, ids.ToStringMap(), "data")
		if err != nil {
			return obj, fmt.Errorf("failed to update ConfigMap data: %w", err)
		}
	}

	// TODO: copy status (not yet serialized)

	return obj, nil
}

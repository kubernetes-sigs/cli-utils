// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Introduces the InventoryConfigMap struct which implements
// the Inventory interface. The InventoryConfigMap wraps a
// ConfigMap resource which stores the set of inventory
// (object metadata).

package inventory

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// WrapInventoryObj takes a passed ConfigMap (as a resource.Info),
// wraps it with the InventoryConfigMap and upcasts the wrapper as
// an the Inventory interface.
func WrapInventoryObj(info *resource.Info) Inventory {
	return &InventoryConfigMap{inv: info}
}

// InventoryConfigMap wraps a ConfigMap resource and implements
// the Inventory interface. This wrapper loads and stores the
// object metadata (inventory) to and from the wrapped ConfigMap.
type InventoryConfigMap struct {
	inv      *resource.Info
	objMetas []object.ObjMetadata
}

// Load is an Inventory interface function returning the set of
// object metadata from the wrapped ConfigMap, or an error.
func (icm *InventoryConfigMap) Load() ([]object.ObjMetadata, error) {
	objs := []object.ObjMetadata{}
	inventoryObj, ok := icm.inv.Object.(*unstructured.Unstructured)
	if !ok {
		err := fmt.Errorf("inventory object is not an Unstructured: %#v", inventoryObj)
		return objs, err
	}
	objMap, exists, err := unstructured.NestedStringMap(inventoryObj.Object, "data")
	if err != nil {
		err := fmt.Errorf("error retrieving object metadata from inventory object")
		return objs, err
	}
	if exists {
		for objStr := range objMap {
			obj, err := object.ParseObjMetadata(objStr)
			if err != nil {
				return objs, err
			}
			objs = append(objs, obj)
		}
	}
	return objs, nil
}

// Store is an Inventory interface function implemented to store
// the object metadata in the wrapped ConfigMap. Actual storing
// happens in "GetObject".
func (icm *InventoryConfigMap) Store(objMetas []object.ObjMetadata) error {
	icm.objMetas = objMetas
	return nil
}

// GetObject returns the wrapped object (ConfigMap) as a resource.Info
// or an error if one occurs.
func (icm *InventoryConfigMap) GetObject() (*resource.Info, error) {
	// Verify the ConfigMap is in Unstructured format.
	obj := icm.inv.Object
	if obj == nil {
		return nil, fmt.Errorf("inventory info has nil Object")
	}
	iot, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf(
			"inventory ConfigMap is not in Unstructured format: %s",
			icm.inv.Source)
	}

	// Create the objMap of all the resources, and compute the hash.
	objMap := buildObjMap(icm.objMetas)
	// Create the inventory object by copying the template.
	invCopy := iot.DeepCopy()
	// Adds the inventory map to the ConfigMap "data" section.
	err := unstructured.SetNestedStringMap(invCopy.UnstructuredContent(),
		objMap, "data")
	if err != nil {
		return nil, err
	}
	return &resource.Info{
		Client:    icm.inv.Client,
		Mapping:   icm.inv.Mapping,
		Source:    "generated",
		Name:      invCopy.GetName(),
		Namespace: invCopy.GetNamespace(),
		Object:    invCopy,
	}, nil
}

func buildObjMap(objMetas []object.ObjMetadata) map[string]string {
	objMap := map[string]string{}
	for _, objMetadata := range objMetas {
		objMap[objMetadata.String()] = ""
	}
	return objMap
}

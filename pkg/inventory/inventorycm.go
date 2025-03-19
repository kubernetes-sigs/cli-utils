// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Introduces the ConfigMap struct which implements
// the Inventory interface. The ConfigMap wraps a
// ConfigMap resource which stores the set of inventory
// (object metadata).

package inventory

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var ConfigMapGVK = schema.GroupVersionKind{
	Group:   "",
	Kind:    "ConfigMap",
	Version: "v1",
}

// ConfigMapToInventoryObj takes a passed ConfigMap (as a resource.Info),
// wraps it with the ConfigMap and upcasts the wrapper as
// an the Inventory interface.
func ConfigMapToInventoryObj(inv *unstructured.Unstructured) (Inventory, error) {
	return configMapToInventory(inv)
}

// ConfigMapToInventoryInfo takes a passed ConfigMap (as a resource.Info),
// wraps it with the ConfigMap and upcasts the wrapper as
// an the Info interface.
func ConfigMapToInventoryInfo(inv *unstructured.Unstructured) (Info, error) {
	return configMapToInventory(inv)
}

// buildDataMap converts the inventory to the storage format to be used in a ConfigMap
func buildDataMap(objMetas object.ObjMetadataSet, objStatus []actuation.ObjectStatus) map[string]string {
	objMap := map[string]string{}
	objStatusMap := map[object.ObjMetadata]actuation.ObjectStatus{}
	for _, status := range objStatus {
		objStatusMap[ObjMetadataFromObjectReference(status.ObjectReference)] = status
	}
	for _, objMetadata := range objMetas {
		if status, found := objStatusMap[objMetadata]; found {
			objMap[objMetadata.String()] = stringFrom(status)
		} else {
			// It's possible that the passed in status doesn't any object status
			objMap[objMetadata.String()] = ""
		}
	}
	return objMap
}

var _ ToUnstructuredFunc = inventoryToConfigMap
var _ FromUnstructuredFunc = configMapToInventory

func configMapToInventory(configMap *unstructured.Unstructured) (*UnstructuredInventory, error) {
	inv := &UnstructuredInventory{
		ClusterObj: configMap,
	}
	objMap, exists, err := unstructured.NestedStringMap(configMap.Object, "data")
	if err != nil {
		err := fmt.Errorf("error retrieving object metadata from inventory object")
		return nil, err
	}
	if exists {
		for objStr := range objMap {
			obj, err := object.ParseObjMetadata(objStr)
			if err != nil {
				return nil, err
			}
			inv.Objs = append(inv.Objs, obj)
		}
	}
	return inv, nil
}

func inventoryToConfigMap(inv *UnstructuredInventory) (*unstructured.Unstructured, error) {
	newConfigMap := inv.ClusterObj.DeepCopy()
	dataMap := buildDataMap(inv.ObjectRefs(), inv.ObjectStatuses())
	// Adds the inventory map to the ConfigMap "data" section.
	err := unstructured.SetNestedStringMap(newConfigMap.UnstructuredContent(),
		dataMap, "data")
	if err != nil {
		return nil, err
	}
	return newConfigMap, err
}

func stringFrom(status actuation.ObjectStatus) string {
	tmp := map[string]string{
		"strategy":  status.Strategy.String(),
		"actuation": status.Actuation.String(),
		"reconcile": status.Reconcile.String(),
	}
	data, err := json.Marshal(tmp)
	if err != nil || string(data) == "{}" {
		return ""
	}
	return string(data)
}

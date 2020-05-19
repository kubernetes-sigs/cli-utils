// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"fmt"
	"sort"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func WrapInventoryObj(info *resource.Info) Inventory {
	return &InventoryConfigMap{inv: info}
}

type InventoryConfigMap struct {
	inv      *resource.Info
	objMetas []*object.ObjMetadata
}

func (icm *InventoryConfigMap) Load() ([]*object.ObjMetadata, error) {
	objs := []*object.ObjMetadata{}
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

func (icm *InventoryConfigMap) Store(objMetas []*object.ObjMetadata) error {
	icm.objMetas = objMetas
	return nil
}

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
	invHashStr, err := computeInventoryHash(objMap)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%s", icm.inv.Name, invHashStr)

	// Create the inventory object by copying the template.
	invCopy := iot.DeepCopy()
	invCopy.SetName(name)
	// Adds the inventory map to the ConfigMap "data" section.
	err = unstructured.SetNestedStringMap(invCopy.UnstructuredContent(),
		objMap, "data")
	if err != nil {
		return nil, err
	}
	annotations := invCopy.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[common.InventoryHash] = invHashStr
	invCopy.SetAnnotations(annotations)

	return &resource.Info{
		Client:    icm.inv.Client,
		Mapping:   icm.inv.Mapping,
		Source:    "generated",
		Name:      invCopy.GetName(),
		Namespace: invCopy.GetNamespace(),
		Object:    invCopy,
	}, nil
}

func buildObjMap(objMetas []*object.ObjMetadata) map[string]string {
	objMap := map[string]string{}
	for _, objMetadata := range objMetas {
		objMap[objMetadata.String()] = ""
	}
	return objMap
}

func computeInventoryHash(objMap map[string]string) (string, error) {
	objList := mapKeysToSlice(objMap)
	sort.Strings(objList)
	invHash, err := calcInventoryHash(objList)
	if err != nil {
		return "", err
	}
	// Compute the name of the inventory object. It is the name of the
	// inventory object template that it is based on with an additional
	// suffix which is based on the hash of the inventory.
	return strconv.FormatUint(uint64(invHash), 16), nil
}

// mapKeysToSlice returns the map keys as a slice of strings.
func mapKeysToSlice(m map[string]string) []string {
	s := make([]string, len(m))
	i := 0
	for k := range m {
		s[i] = k
		i++
	}
	return s
}

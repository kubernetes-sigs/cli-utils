// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code for a "inventory" object which
// stores object metadata to keep track of sets of
// resources. This "inventory" object must be a ConfigMap
// and it stores the object metadata in the data field
// of the ConfigMap. By storing metadata from all applied
// objects, we can correctly prune and teardown sets
// of resources.

package inventory

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The default inventory name stored in the inventory template.
const legacyInvName = "inventory"

// FindInventoryObj returns the "Inventory" object (ConfigMap with
// inventory label) if it exists, or nil if it does not exist.
func FindInventoryObj(objs object.UnstructuredSet) *unstructured.Unstructured {
	for _, obj := range objs {
		if obj != nil && IsInventoryObject(obj) {
			return obj
		}
	}
	return nil
}

// IsInventoryObject returns true if the passed object has the
// inventory label.
func IsInventoryObject(obj client.Object) bool {
	if obj == nil {
		return false
	}
	if len(InventoryLabel(obj)) > 0 {
		return true
	}
	return false
}

// InventoryLabel returns the string value of the InventoryLabel
// for the passed inventory object. Returns empty string if not found.
func InventoryLabel(obj client.Object) string {
	labels := obj.GetLabels()
	if labels == nil {
		return ""
	}
	return labels[common.InventoryLabel]
}

// SetInventoryLabel updates the string value of the InventoryLabel
// for the passed inventory object.
func SetInventoryLabel(obj client.Object, id string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string, 1)
	}
	labels[common.InventoryLabel] = id
	obj.SetLabels(labels)
}

// ValidateNoInventory takes a set of unstructured.Unstructured objects and
// validates that no inventory object is in the input slice.
func ValidateNoInventory(objs object.UnstructuredSet) error {
	invs := make(object.UnstructuredSet, 0)
	for _, obj := range objs {
		if IsInventoryObject(obj) {
			invs = append(invs, obj)
		}
	}
	if len(invs) == 0 {
		return nil
	}
	return MultipleInventoryObjError{
		InventoryObjectTemplates: invs,
	}
}

// splitUnstructureds takes a set of unstructured.Unstructured objects and
// splits it into one set that contains the inventory object templates and
// another one that contains the remaining resources. If there is no inventory
// object the first return value is nil. Returns an error if there are
// more than one inventory objects.
func SplitUnstructureds(objs object.UnstructuredSet) (*unstructured.Unstructured, object.UnstructuredSet, error) {
	invs := make(object.UnstructuredSet, 0)
	resources := make(object.UnstructuredSet, 0)
	for _, obj := range objs {
		if IsInventoryObject(obj) {
			invs = append(invs, obj)
		} else {
			resources = append(resources, obj)
		}
	}
	var inv *unstructured.Unstructured
	var err error
	if len(invs) == 1 {
		inv = invs[0]
	} else if len(invs) > 1 {
		err = MultipleInventoryObjError{
			InventoryObjectTemplates: invs,
		}
	}
	return inv, resources, err
}

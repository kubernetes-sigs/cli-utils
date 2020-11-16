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
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// The default inventory name stored in the inventory template.
const legacyInvName = "inventory"

// Inventory describes methods necessary for an object which
// can persist the object metadata for pruning and other group
// operations.
type Inventory interface {
	// Load retrieves the set of object metadata from the inventory object
	Load() ([]object.ObjMetadata, error)
	// Store the set of object metadata in the inventory object
	Store(objs []object.ObjMetadata) error
	// GetObject returns the object that stores the inventory
	GetObject() (*unstructured.Unstructured, error)
}

// InventoryFactoryFunc creates the object which implements the Inventory
// interface from the passed info object.
type InventoryFactoryFunc func(*unstructured.Unstructured) Inventory

// InventoryToUnstructuredFunc returns the unstructured object for the
// given InventoryInfo.
type InventoryToUnstructuredFunc func(InventoryInfo) *unstructured.Unstructured

// FindInventoryObj returns the "Inventory" object (ConfigMap with
// inventory label) if it exists, or nil if it does not exist.
func FindInventoryObj(objs []*unstructured.Unstructured) *unstructured.Unstructured {
	for _, obj := range objs {
		if IsInventoryObject(obj) {
			return obj
		}
	}
	return nil
}

// IsInventoryObject returns true if the passed object has the
// inventory label.
func IsInventoryObject(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	inventoryLabel, err := retrieveInventoryLabel(obj)
	if err == nil && len(inventoryLabel) > 0 {
		return true
	}
	return false
}

// retrieveInventoryLabel returns the string value of the InventoryLabel
// for the passed inventory object. Returns error if the passed object is nil or
// is not a inventory object.
func retrieveInventoryLabel(obj *unstructured.Unstructured) (string, error) {
	if obj == nil {
		return "", fmt.Errorf("inventory info is nil")
	}

	accessor, err := meta.Accessor(obj)
	if err != nil {
		return "", err
	}
	labels := accessor.GetLabels()
	inventoryLabel, exists := labels[common.InventoryLabel]
	if !exists {
		return "", fmt.Errorf("inventory label does not exist for inventory object: %s", common.InventoryLabel)
	}
	return strings.TrimSpace(inventoryLabel), nil
}

// ValidateNoInventory takes a slice of unstructured.Unstructured objects and
// validates that no inventory object is in the input slice.
func ValidateNoInventory(objs []*unstructured.Unstructured) error {
	invs := make([]*unstructured.Unstructured, 0)
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

// splitUnstructureds takes a slice of unstructured.Unstructured objects and
// splits it into one slice that contains the inventory object templates and
// another one that contains the remaining resources.
func SplitUnstructureds(objs []*unstructured.Unstructured) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	invs := make([]*unstructured.Unstructured, 0)
	resources := make([]*unstructured.Unstructured, 0)
	for _, obj := range objs {
		if IsInventoryObject(obj) {
			invs = append(invs, obj)
		} else {
			resources = append(resources, obj)
		}
	}
	if len(invs) == 0 {
		return nil, resources, NoInventoryObjError{}
	} else if len(invs) > 1 {
		return nil, resources, MultipleInventoryObjError{
			InventoryObjectTemplates: invs,
		}
	}
	return invs[0], resources, nil
}

// addSuffixToName adds the passed suffix (usually a hash) as a suffix
// to the name of the passed object stored in the Info struct. Returns
// an error if name stored in the object differs from the name in
// the Info struct.
func addSuffixToName(obj *unstructured.Unstructured, suffix string) error {
	if obj == nil {
		return fmt.Errorf("nil unstructured.Unstructured")
	}
	suffix = strings.TrimSpace(suffix)
	if len(suffix) == 0 {
		return fmt.Errorf("passed empty suffix")
	}

	accessor, _ := meta.Accessor(obj)
	name := accessor.GetName()
	if name != obj.GetName() {
		return fmt.Errorf("inventory object (%s) and resource.Info (%s) have different names", name, obj.GetName())
	}
	// Error if name already has suffix.
	suffix = "-" + suffix
	if strings.HasSuffix(name, suffix) {
		return fmt.Errorf("name already has suffix: %s", name)
	}
	name += suffix
	accessor.SetName(name)

	return nil
}

// fixLegacyInventoryName modifies the inventory name if it is
// the legacy default name (i.e. inventory) by adding a random suffix.
// This fixes a problem where inventory object names collide if
// they are created in the same namespace.
func fixLegacyInventoryName(obj *unstructured.Unstructured) error {
	if obj == nil {
		return fmt.Errorf("invalid inventory object is nil")
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	name := accessor.GetName()
	if obj.GetName() == legacyInvName || name == legacyInvName {
		klog.V(4).Infof("renaming legacy inventory name")
		seed := time.Now().UTC().UnixNano()
		randomSuffix := common.RandomStr(seed)
		return addSuffixToName(obj, randomSuffix)
	}
	return nil
}

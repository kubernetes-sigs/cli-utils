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
	"k8s.io/cli-runtime/pkg/resource"
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
	GetObject() (*resource.Info, error)
}

// FindInventoryObj returns the "Inventory" object (ConfigMap with
// inventory label) if it exists, or nil if it does not exist.
func FindInventoryObj(infos []*resource.Info) *resource.Info {
	for _, info := range infos {
		if IsInventoryObject(info) {
			return info
		}
	}
	return nil
}

// IsInventoryObject returns true if the passed object has the
// inventory label.
func IsInventoryObject(info *resource.Info) bool {
	if info == nil {
		return false
	}
	inventoryLabel, err := retrieveInventoryLabel(info)
	if err == nil && len(inventoryLabel) > 0 {
		return true
	}
	return false
}

// retrieveInventoryLabel returns the string value of the InventoryLabel
// for the passed inventory object. Returns error if the passed object is nil or
// is not a inventory object.
func retrieveInventoryLabel(info *resource.Info) (string, error) {
	if info == nil {
		return "", fmt.Errorf("inventory info is nil")
	}
	obj := info.Object
	if obj == nil {
		return "", fmt.Errorf("inventory object is nil")
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

// splitInfos takes a slice of resource.Info objects and splits it
// into one slice that contains the inventory object templates and
// another one that contains the remaining resources.
func SplitInfos(infos []*resource.Info) (*resource.Info, []*resource.Info, error) {
	invs := make([]*resource.Info, 0)
	resources := make([]*resource.Info, 0)
	for _, info := range infos {
		if IsInventoryObject(info) {
			invs = append(invs, info)
		} else {
			resources = append(resources, info)
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

// ClearInventoryObj finds the inventory object in the list of objects,
// and sets an empty inventory. Returns an error if once occurred.
func ClearInventoryObj(invInfo *resource.Info) (*resource.Info, error) {
	if invInfo == nil {
		return nil, fmt.Errorf("clearing nil inventory object")
	}
	if !IsInventoryObject(invInfo) {
		return nil, fmt.Errorf("attempting to clear non-inventory object")
	}
	wrapped := WrapInventoryObj(invInfo)
	if err := wrapped.Store([]object.ObjMetadata{}); err != nil {
		return nil, err
	}
	return wrapped.GetObject()
}

// addSuffixToName adds the passed suffix (usually a hash) as a suffix
// to the name of the passed object stored in the Info struct. Returns
// an error if name stored in the object differs from the name in
// the Info struct.
func addSuffixToName(info *resource.Info, suffix string) error {
	if info == nil {
		return fmt.Errorf("nil resource.Info")
	}
	suffix = strings.TrimSpace(suffix)
	if len(suffix) == 0 {
		return fmt.Errorf("passed empty suffix")
	}

	accessor, _ := meta.Accessor(info.Object)
	name := accessor.GetName()
	if name != info.Name {
		return fmt.Errorf("inventory object (%s) and resource.Info (%s) have different names", name, info.Name)
	}
	// Error if name already has suffix.
	suffix = "-" + suffix
	if strings.HasSuffix(name, suffix) {
		return fmt.Errorf("name already has suffix: %s", name)
	}
	name += suffix
	accessor.SetName(name)
	info.Name = name

	return nil
}

// fixLegacyInventoryName modifies the inventory name if it is
// the legacy default name (i.e. inventory) by adding a random suffix.
// This fixes a problem where inventory object names collide if
// they are created in the same namespace.
func fixLegacyInventoryName(info *resource.Info) error {
	if info == nil || info.Object == nil {
		return fmt.Errorf("invalid inventory object is nil or info.Object is nil")
	}
	obj := info.Object
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	name := accessor.GetName()
	if info.Name == legacyInvName || name == legacyInvName {
		klog.V(4).Infof("renaming legacy inventory name")
		seed := time.Now().UTC().UnixNano()
		randomSuffix := common.RandomStr(seed)
		return addSuffixToName(info, randomSuffix)
	}
	return nil
}

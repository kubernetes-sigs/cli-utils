// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// InventoryPolicy defines if an inventory object can take over
// objects that belong to another inventory object or don't
// belong to any inventory object.
// This is done by determining if the apply/prune operation
// can go through for a resource based on the comparison
// the inventory-id value in the package and the owning-inventory
// annotation in the live object.
type InventoryPolicy int

const (
	// InvnetoryPolicyMustMatch: This policy enforces that the resources being applied can not
	// have any overlap with objects in other inventories or objects that already exist
	// in the cluster but don't belong to an inventory.
	//
	// The apply operation can go through when
	// - A new resources in the package doesn't exist in the cluster
	// - An existing resource in the package doesn't exist in the cluster
	// - An existing resource exist in the cluster. The owning-inventory annotation in the live object
	//   matches with that in the package.
	//
	// The prune operation can go through when
	// - The owning-inventory annotation in the live object match with that
	//   in the package.
	InventoryPolicyMustMatch InventoryPolicy = iota

	// AdoptIfNoInventory: This policy enforces that resources being applied
	// can not have any overlap with objects in other inventories, but are
	// permitted to take ownership of objects that don't belong to any inventories.
	//
	// The apply operation can go through when
	// - New resource in the package doesn't exist in the cluster
	// - If a new resource exist in the cluster, its owning-inventory annotation is empty
	// - Existing resource in the package doesn't exist in the cluster
	// - If existing resource exist in the cluster, its owning-inventory annotation in the live object
	//   is empty
	// - An existing resource exist in the cluster. The owning-inventory annotation in the live object
	//   matches with that in the package.
	//
	// The prune operation can go through when
	// - The owning-inventory annotation in the live object match with that
	//   in the package.
	// - The live object doesn't have the owning-inventory annotation.
	AdoptIfNoInventory

	// AdoptAll: This policy will let the current inventory take ownership of any objects.
	//
	// The apply operation can go through for any resource in the package even if the
	// live object has an unmatched owning-inventory annotation.
	//
	// The prune operation can go through when
	// - The owning-inventory annotation in the live object match or doesn't match with that
	//   in the package.
	// - The live object doesn't have the owning-inventory annotation.
	AdoptAll
)

const owningInventoryKey = "config.k8s.io/owning-inventory"

// inventoryIDMatchStatus represents the result of comparing the
// id from current inventory info and the inventory-id from a live object.
type inventoryIDMatchStatus int

const (
	Empty inventoryIDMatchStatus = iota
	Match
	NoMatch
)

func inventoryIDMatch(inv InventoryInfo, obj *unstructured.Unstructured) inventoryIDMatchStatus {
	annotations := obj.GetAnnotations()
	value, found := annotations[owningInventoryKey]
	if !found {
		return Empty
	}
	if value == inv.ID() {
		return Match
	}
	return NoMatch
}

func CanApply(inv InventoryInfo, obj *unstructured.Unstructured, policy InventoryPolicy) (bool, error) {
	if obj == nil {
		return true, nil
	}
	matchStatus := inventoryIDMatch(inv, obj)
	switch matchStatus {
	case Empty:
		if policy != InventoryPolicyMustMatch {
			return true, nil
		}
		err := fmt.Errorf("can't adopt an object without the annotation %s", owningInventoryKey)
		return false, NewNeedAdoptionError(err)
	case Match:
		return true, nil
	case NoMatch:
		if policy == AdoptAll {
			return true, nil
		}
		err := fmt.Errorf("can't apply the resource since its annotation %s is a different inventory object", owningInventoryKey)
		return false, NewInventoryOverlapError(err)
	}
	// shouldn't reach here
	return false, nil
}

func CanPrune(inv InventoryInfo, obj *unstructured.Unstructured, policy InventoryPolicy) bool {
	if obj == nil {
		return false
	}
	matchStatus := inventoryIDMatch(inv, obj)
	switch matchStatus {
	case Empty:
		return policy == AdoptIfNoInventory || policy == AdoptAll
	case Match:
		return true
	case NoMatch:
		return policy == AdoptAll
	}
	return false
}

func AddInventoryIDAnnotation(obj *unstructured.Unstructured, inv InventoryInfo) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[owningInventoryKey] = inv.ID()
	obj.SetAnnotations(annotations)
}

type InventoryOverlapError struct {
	err error
}

func (e *InventoryOverlapError) Error() string {
	return e.err.Error()
}

func NewInventoryOverlapError(err error) *InventoryOverlapError {
	return &InventoryOverlapError{err: err}
}

type NeedAdoptionError struct {
	err error
}

func (e *NeedAdoptionError) Error() string {
	return e.err.Error()
}

func NewNeedAdoptionError(err error) *NeedAdoptionError {
	return &NeedAdoptionError{err: err}
}

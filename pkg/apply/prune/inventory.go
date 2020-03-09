// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Inventory encapsulates a set of ObjMetadata structs,
// providing easy functionality to manipulate these sets.

package prune

import (
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/cli-utils/pkg/object"
)

// Inventory encapsulates a grouping of unique Inventory
// structs. Organizes the Inventory structs with a map,
// which ensures there are no duplicates. Allows set
// operations such as merging sets and subtracting sets.
type Inventory struct {
	set map[string]*object.ObjMetadata
}

// NewInventory returns a pointer to an Inventory
// struct grouping the passed Inventory items.
func NewInventory(items []*object.ObjMetadata) *Inventory {
	inventory := Inventory{set: map[string]*object.ObjMetadata{}}
	inventory.AddItems(items)
	return &inventory
}

// GetItems returns the set of pointers to ObjMetadata
// structs.
func (is *Inventory) GetItems() []*object.ObjMetadata {
	items := []*object.ObjMetadata{}
	for _, item := range is.set {
		items = append(items, item)
	}
	return items
}

// AddItems adds Inventory structs to the set which
// are not already in the set.
func (is *Inventory) AddItems(items []*object.ObjMetadata) {
	for _, item := range items {
		if item != nil {
			is.set[item.String()] = item
		}
	}
}

// DeleteItem removes an ObjMetadata struct from the
// set if it exists in the set. Returns true if the
// ObjMetadata item was deleted, false if it did not exist
// in the set.
func (is *Inventory) DeleteItem(item *object.ObjMetadata) bool {
	if item == nil {
		return false
	}
	if _, ok := is.set[item.String()]; ok {
		delete(is.set, item.String())
		return true
	}
	return false
}

// Merge combines the unique set of ObjMetadata items from the
// current set with the passed "other" set, returning a new
// set or error. Returns an error if the passed set to merge
// is nil.
func (is *Inventory) Merge(other *Inventory) (*Inventory, error) {
	if other == nil {
		return nil, fmt.Errorf("inventory to merge is nil")
	}
	// Copy the current Inventory into result
	result := NewInventory(is.GetItems())
	result.AddItems(other.GetItems())
	return result, nil
}

// Subtract removes the Inventory items in the "other" set from the
// current set, returning a new set. This does not modify the current
// set. Returns an error if the passed set to subtract is nil.
func (is *Inventory) Subtract(other *Inventory) (*Inventory, error) {
	if other == nil {
		return nil, fmt.Errorf("inventory to subtract is nil")
	}
	// Copy the current Inventory into result
	result := NewInventory(is.GetItems())
	// Remove each item in "other" which exists in "result"
	for _, item := range other.GetItems() {
		result.DeleteItem(item)
	}
	return result, nil
}

// Equals returns true if the "other" inventory set is the same
// as this current inventory set. Relies on the fact that the
// inventory items are sorted for the String() function.
func (is *Inventory) Equals(other *Inventory) bool {
	if other == nil {
		return false
	}
	return is.String() == other.String()
}

// String returns a string describing set of ObjMetadata structs.
func (is *Inventory) String() string {
	strs := []string{}
	for _, item := range is.GetItems() {
		strs = append(strs, item.String())
	}
	sort.Strings(strs)
	return strings.Join(strs, ", ")
}

// Size returns the number of ObjMetadata structs in the set.
func (is *Inventory) Size() int {
	return len(is.set)
}

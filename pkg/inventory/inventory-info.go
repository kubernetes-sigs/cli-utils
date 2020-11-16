// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

// InventoryInfo provides the minimal information for the applier
// to create, look up and update an inventory.
// The inventory object can be any type, the Provider in the applier
// needs to know how to create, look up and update it based
// on the InventoryInfo.
type InventoryInfo interface {
	// Namespace of the inventory object.
	// It should be the value of the field .metadata.namespace.
	Namespace() string

	// Name of the inventory object.
	// It should be the value of the field .metadata.name.
	Name() string

	// ID of the inventory object. It is optional.
	// The Provider contained in the applier should know
	// if the Id is necessary and how to use it for pruning objects.
	ID() string
}

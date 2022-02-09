// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
)

// Client provides accessor methods for CRUD operations on Inventories.
// TODO: pass DryRun to Client methods
type Client interface {
	GroupVersionKind() schema.GroupVersionKind
	Load(InventoryInfo) (*actuation.Inventory, error)
	Store(*actuation.Inventory, common.DryRunStrategy) error
	Delete(InventoryInfo, common.DryRunStrategy) error
}

// InventoryInfo provides enough information for the Client to uniquely
// identify an inventory.
type InventoryInfo struct {
	actuation.ObjectReference

	// ID uniquely identifies an inventory object.
	// Used for labelling the inventory and annotating the objects it contains.
	ID string
}

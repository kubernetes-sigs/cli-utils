// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
)

// Client provides accessor methods for CRUD operations on Inventories.
// TODO: pass DryRun to Client methods
type Client interface {
	GroupVersionKind() schema.GroupVersionKind
	Load(context.Context, Info) (*actuation.Inventory, error)
	Store(context.Context, *actuation.Inventory, common.DryRunStrategy) error
	Delete(context.Context, Info, common.DryRunStrategy) error
	List(context.Context, Info) ([]*actuation.Inventory, error)
}

// Info provides enough information for the Client to uniquely
// identify an inventory.
type Info struct {
	actuation.ObjectReference

	// ID uniquely identifies an inventory object.
	// Used for labelling the inventory and annotating the objects it contains.
	ID string
}

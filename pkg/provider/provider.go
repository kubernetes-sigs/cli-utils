// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

// InventoryProvider implements the Provider interface.
var _ Provider = &InventoryProvider{}

// Provider is an interface which wraps the kubectl factory and
// the inventory client.
type Provider interface {
	Factory() util.Factory
	InventoryClient() (inventory.InventoryClient, error)
}

// InventoryProvider implements the Provider interface.
type InventoryProvider struct {
	factory util.Factory
}

// NewProvider returns a Provider that implements a ConfigMap inventory object.
func NewProvider(f util.Factory) *InventoryProvider {
	return &InventoryProvider{
		factory: f,
	}
}

// Factory returns the kubectl factory.
func (f *InventoryProvider) Factory() util.Factory {
	return f.factory
}

// InventoryClient returns an InventoryClient created with the stored
// factory and InventoryFactoryFunc values, or an error if one occurred.
func (f *InventoryProvider) InventoryClient() (inventory.InventoryClient, error) {
	return inventory.NewInventoryClient(f.factory,
		inventory.WrapInventoryObj,
		inventory.InvInfoToConfigMap,
	)
}

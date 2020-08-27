// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

// Provider is an interface which wraps the kubectl factory and
// the inventory client.
type Provider interface {
	Factory() util.Factory
	InventoryClient() (inventory.InventoryClient, error)
	ToRESTMapper() (meta.RESTMapper, error)
}

// InventoryProvider implements the Provider interface.
type InventoryProvider struct {
	factory        util.Factory
	invFactoryFunc inventory.InventoryFactoryFunc
}

// NewProvider encapsulates the passed values, and returns a pointer to an Provider.
func NewProvider(f util.Factory, invFactoryFunc inventory.InventoryFactoryFunc) *InventoryProvider {
	return &InventoryProvider{
		factory:        f,
		invFactoryFunc: invFactoryFunc,
	}
}

// Factory returns the kubectl factory.
func (f *InventoryProvider) Factory() util.Factory {
	return f.factory
}

// InventoryClient returns an InventoryClient created with the stored
// factory and InventoryFactoryFunc values, or an error if one occurred.
func (f *InventoryProvider) InventoryClient() (inventory.InventoryClient, error) {
	return inventory.NewInventoryClient(f.factory, f.invFactoryFunc)
}

// ToRESTMapper returns a RESTMapper created by the stored kubectl factory.
func (f *InventoryProvider) ToRESTMapper() (meta.RESTMapper, error) {
	return f.factory.ToRESTMapper()
}

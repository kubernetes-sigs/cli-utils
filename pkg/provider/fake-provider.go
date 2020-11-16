// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type FakeProvider struct {
	factory   util.Factory
	InvClient *inventory.FakeInventoryClient
}

var _ Provider = &FakeProvider{}

func NewFakeProvider(f util.Factory, objs []object.ObjMetadata) *FakeProvider {
	return &FakeProvider{
		factory:   f,
		InvClient: inventory.NewFakeInventoryClient(objs),
	}
}

func (f *FakeProvider) Factory() util.Factory {
	return f.factory
}

func (f *FakeProvider) InventoryClient() (inventory.InventoryClient, error) {
	return f.InvClient, nil
}

func (f *FakeProvider) ToRESTMapper() (meta.RESTMapper, error) {
	return f.factory.ToRESTMapper()
}

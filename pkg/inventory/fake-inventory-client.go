// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// FakeInventoryClient is a testing implementation of the InventoryClient interface.
type FakeInventoryClient struct {
	objs []object.ObjMetadata
	err  error
}

var _ InventoryClient = &FakeInventoryClient{}

// NewFakeInventoryClient returns a FakeInventoryClient.
func NewFakeInventoryClient(initObjs []object.ObjMetadata) *FakeInventoryClient {
	return &FakeInventoryClient{
		objs: initObjs,
		err:  nil,
	}
}

// GetClusterObjs returns currently stored set of objects.
func (fic *FakeInventoryClient) GetClusterObjs(inv *resource.Info) ([]object.ObjMetadata, error) {
	if fic.err != nil {
		return []object.ObjMetadata{}, fic.err
	}
	return fic.objs, nil
}

// Merge stores the passed objects with the current stored cluster inventory
// objects. Returns the set difference of the current set of objects minus
// the passed set of objects, or an error if one is set up.
func (fic *FakeInventoryClient) Merge(inv *resource.Info, objs []object.ObjMetadata) ([]object.ObjMetadata, error) {
	if fic.err != nil {
		return []object.ObjMetadata{}, fic.err
	}
	diffObjs := object.SetDiff(fic.objs, objs)
	fic.objs = object.Union(fic.objs, objs)
	return diffObjs, nil
}

// Replace the stored cluster inventory objs with the passed obj, or an
// error if one is set up.
func (fic *FakeInventoryClient) Replace(inv *resource.Info, objs []object.ObjMetadata) error {
	if fic.err != nil {
		return fic.err
	}
	fic.objs = objs
	return nil
}

// DeleteInventoryObj returns an error if one is forced; does nothing otherwise.
func (fic *FakeInventoryClient) DeleteInventoryObj(inv *resource.Info) error {
	if fic.err != nil {
		return fic.err
	}
	return nil
}

// ClearInventoryObj implements InventoryClient interface function. It does nothing for now.
func (fic *FakeInventoryClient) ClearInventoryObj(inv *resource.Info) (*resource.Info, error) {
	return inv, nil
}

func (fic *FakeInventoryClient) SetDryRunStrategy(drs common.DryRunStrategy) {}

func (fic *FakeInventoryClient) SetInventoryFactoryFunc(fn InventoryFactoryFunc) {}

// SetError forces an error on the subsequent client call if it returns an error.
func (fic *FakeInventoryClient) SetError(err error) {
	fic.err = err
}

// ClearError clears the force error
func (fic *FakeInventoryClient) ClearError() {
	fic.err = nil
}

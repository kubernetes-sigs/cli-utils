// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// FakeInventoryClient is a testing implementation of the InventoryClient interface.
type FakeInventoryClient struct {
	Objs []object.ObjMetadata
	Err  error
}

var _ InventoryClient = &FakeInventoryClient{}

// NewFakeInventoryClient returns a FakeInventoryClient.
func NewFakeInventoryClient(initObjs []object.ObjMetadata) *FakeInventoryClient {
	return &FakeInventoryClient{
		Objs: initObjs,
		Err:  nil,
	}
}

// GetClusterObjs returns currently stored set of objects.
func (fic *FakeInventoryClient) GetClusterObjs(inv InventoryInfo) ([]object.ObjMetadata, error) {
	if fic.Err != nil {
		return []object.ObjMetadata{}, fic.Err
	}
	return fic.Objs, nil
}

// Merge stores the passed objects with the current stored cluster inventory
// objects. Returns the set difference of the current set of objects minus
// the passed set of objects, or an error if one is set up.
func (fic *FakeInventoryClient) Merge(inv InventoryInfo, objs []object.ObjMetadata) ([]object.ObjMetadata, error) {
	if fic.Err != nil {
		return []object.ObjMetadata{}, fic.Err
	}
	diffObjs := object.SetDiff(fic.Objs, objs)
	fic.Objs = object.Union(fic.Objs, objs)
	return diffObjs, nil
}

// Replace the stored cluster inventory objs with the passed obj, or an
// error if one is set up.
func (fic *FakeInventoryClient) Replace(inv InventoryInfo, objs []object.ObjMetadata) error {
	if fic.Err != nil {
		return fic.Err
	}
	fic.Objs = objs
	return nil
}

// DeleteInventoryObj returns an error if one is forced; does nothing otherwise.
func (fic *FakeInventoryClient) DeleteInventoryObj(inv InventoryInfo) error {
	if fic.Err != nil {
		return fic.Err
	}
	return nil
}

func (fic *FakeInventoryClient) SetDryRunStrategy(drs common.DryRunStrategy) {}

func (fic *FakeInventoryClient) ApplyInventoryNamespace(inv *unstructured.Unstructured) error {
	if fic.Err != nil {
		return fic.Err
	}
	return nil
}

// SetError forces an error on the subsequent client call if it returns an error.
func (fic *FakeInventoryClient) SetError(err error) {
	fic.Err = err
}

// ClearError clears the force error
func (fic *FakeInventoryClient) ClearError() {
	fic.Err = nil
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// FakeInventoryClient is a testing implementation of the InventoryClient interface.
type FakeInventoryClient struct {
	Objs object.ObjMetadataSet
	Err  error
}

var (
	_ InventoryClient        = &FakeInventoryClient{}
	_ InventoryClientFactory = FakeInventoryClientFactory{}
)

type FakeInventoryClientFactory object.ObjMetadataSet

func (f FakeInventoryClientFactory) NewInventoryClient(factory cmdutil.Factory) (InventoryClient, error) {
	return NewFakeInventoryClient(object.ObjMetadataSet(f)), nil
}

// NewFakeInventoryClient returns a FakeInventoryClient.
func NewFakeInventoryClient(initObjs object.ObjMetadataSet) *FakeInventoryClient {
	return &FakeInventoryClient{
		Objs: initObjs,
		Err:  nil,
	}
}

// GetClusterObjs returns currently stored set of objects.
func (fic *FakeInventoryClient) GetClusterObjs(InventoryInfo, common.DryRunStrategy) (object.ObjMetadataSet, error) {
	if fic.Err != nil {
		return object.ObjMetadataSet{}, fic.Err
	}
	return fic.Objs, nil
}

// Merge stores the passed objects with the current stored cluster inventory
// objects. Returns the set difference of the current set of objects minus
// the passed set of objects, or an error if one is set up.
func (fic *FakeInventoryClient) Merge(_ InventoryInfo, objs object.ObjMetadataSet, _ common.DryRunStrategy) (object.ObjMetadataSet, error) {
	if fic.Err != nil {
		return object.ObjMetadataSet{}, fic.Err
	}
	diffObjs := fic.Objs.Diff(objs)
	fic.Objs = fic.Objs.Union(objs)
	return diffObjs, nil
}

// Replace the stored cluster inventory objs with the passed obj, or an
// error if one is set up.

func (fic *FakeInventoryClient) Replace(_ InventoryInfo, objs object.ObjMetadataSet, _ common.DryRunStrategy) error {
	if fic.Err != nil {
		return fic.Err
	}
	fic.Objs = objs
	return nil
}

// DeleteInventoryObj returns an error if one is forced; does nothing otherwise.
func (fic *FakeInventoryClient) DeleteInventoryObj(InventoryInfo, common.DryRunStrategy) error {
	if fic.Err != nil {
		return fic.Err
	}
	return nil
}

func (fic *FakeInventoryClient) ApplyInventoryNamespace(*unstructured.Unstructured, common.DryRunStrategy) error {
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

func (fic *FakeInventoryClient) GetClusterInventoryInfo(InventoryInfo, common.DryRunStrategy) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (fic *FakeInventoryClient) GetClusterInventoryObjs(_ InventoryInfo) (object.UnstructuredSet, error) {
	return object.UnstructuredSet{}, nil
}

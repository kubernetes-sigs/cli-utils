// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// FakeClient is a testing implementation of the inventory.Client interface.
type FakeClient struct {
	Objs object.ObjMetadataSet
	Err  error
}

var _ Client = &FakeClient{}

// NewFakeClient returns a FakeClient.
func NewFakeClient(initObjs object.ObjMetadataSet) *FakeClient {
	return &FakeClient{
		Objs: initObjs,
		Err:  nil,
	}
}

func (fc *FakeClient) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "fake",
		Version: "fake",
		Kind:    "fake",
	}
}

func (fc *FakeClient) Load(_ context.Context, invInfo Info) (*actuation.Inventory, error) {
	if fc.Err != nil {
		return nil, fc.Err
	}
	return &actuation.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name: invInfo.Name,
		},
		Spec: actuation.InventorySpec{
			Objects: ObjectReferencesFromObjMetadataSet(fc.Objs),
		},
	}, nil
}

func (fc *FakeClient) List(_ context.Context, invInfo Info) ([]*actuation.Inventory, error) {
	if fc.Err != nil {
		return nil, fc.Err
	}

	return []*actuation.Inventory{{
		ObjectMeta: metav1.ObjectMeta{
			Name: invInfo.Name,
		},
		Spec: actuation.InventorySpec{
			Objects: ObjectReferencesFromObjMetadataSet(fc.Objs),
		},
	}}, nil
}

// Store the stored cluster inventory objs with the passed inv.Spec.Objects,
// or error if one is set up.
func (fc *FakeClient) Store(_ context.Context, inv *actuation.Inventory, _ common.DryRunStrategy) error {
	if fc.Err != nil {
		return fc.Err
	}
	fc.Objs = ObjMetadataSetFromObjectReferences(inv.Spec.Objects)
	return nil
}

// Delete returns an error if one is forced; does nothing otherwise.
func (fc *FakeClient) Delete(_ context.Context, _ Info, _ common.DryRunStrategy) error {
	if fc.Err != nil {
		return fc.Err
	}
	return nil
}

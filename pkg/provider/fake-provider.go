// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"io"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
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

func (f *FakeProvider) ManifestReader(reader io.Reader, _ []string) (manifestreader.ManifestReader, error) {
	mapper, err := f.factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}

	readerOptions := manifestreader.ReaderOptions{
		Mapper:    mapper,
		Namespace: metav1.NamespaceDefault,
	}
	return &manifestreader.StreamManifestReader{
		ReaderName:    "stdin",
		Reader:        reader,
		ReaderOptions: readerOptions,
	}, nil
}

func (f *FakeProvider) InventoryInfo(objs []*unstructured.Unstructured) (inventory.InventoryInfo, []*unstructured.Unstructured, error) {
	return inventory.SplitUnstructureds(objs)
}

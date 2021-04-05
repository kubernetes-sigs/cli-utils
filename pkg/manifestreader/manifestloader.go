// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

// ManifestLoader is an interface for reading
// and parsing the resources and inventory info.
type ManifestLoader interface {
	InventoryInfo([]*unstructured.Unstructured) (inventory.InventoryInfo, []*unstructured.Unstructured, error)
	ManifestReader(reader io.Reader, path string) (ManifestReader, error)
}

// manifestLoader implements the ManifestLoader interface
// for ConfigMap as the inventory object.
type manifestLoader struct {
	factory util.Factory
}

// NewManifestLoader returns an instance of manifestLoader.
func NewManifestLoader(f util.Factory) ManifestLoader {
	return &manifestLoader{
		factory: f,
	}
}

// InventoryInfo returns the InventoryInfo from a list of Unstructured objects.
func (f *manifestLoader) InventoryInfo(objs []*unstructured.Unstructured) (inventory.InventoryInfo, []*unstructured.Unstructured, error) {
	invObj, objs, err := inventory.SplitUnstructureds(objs)
	return inventory.WrapInventoryInfoObj(invObj), objs, err
}

func (f *manifestLoader) ManifestReader(reader io.Reader, path string) (ManifestReader, error) {
	// Fetch the namespace from the configloader. The source of this
	// either the namespace flag or the context. If the namespace is provided
	// with the flag, enforceNamespace will be true. In this case, it is
	// an error if any of the resources in the package has a different
	// namespace set.
	namespace, enforceNamespace, err := f.factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return nil, err
	}

	mapper, err := f.factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}

	readerOptions := ReaderOptions{
		Mapper:           mapper,
		Namespace:        namespace,
		EnforceNamespace: enforceNamespace,
	}

	return mReader(path, reader, readerOptions), nil
}

// mReader returns the ManifestReader based in the input args
func mReader(path string, reader io.Reader, readerOptions ReaderOptions) ManifestReader {
	var mReader ManifestReader
	// Read from stdin if "-" is specified, similar to kubectl
	if path == "-" {
		mReader = &StreamManifestReader{
			ReaderName:    "stdin",
			Reader:        reader,
			ReaderOptions: readerOptions,
		}
	} else {
		mReader = &PathManifestReader{
			Path:          path,
			ReaderOptions: readerOptions,
		}
	}
	return mReader
}

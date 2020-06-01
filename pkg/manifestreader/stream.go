// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"io"

	"k8s.io/cli-runtime/pkg/resource"
)

// StreamManifestReader reads manifest from the provided io.Reader
// and returns them as Info objects. The returned Infos will not have
// client or mapping set.
type StreamManifestReader struct {
	ReaderName string
	Reader     io.Reader

	ReaderOptions
}

// Read reads the manifests and returns them as Info objects.
func (r *StreamManifestReader) Read() ([]*resource.Info, error) {
	validator, err := r.Factory.Validator(r.Validate)
	if err != nil {
		return nil, err
	}

	result := r.Factory.NewBuilder().
		Local().
		Unstructured().
		Schema(validator).
		ContinueOnError().
		Stream(r.Reader, r.ReaderName).
		Flatten().
		Do()

	if err := result.Err(); err != nil {
		return nil, err
	}
	infos, err := result.Infos()
	if err != nil {
		return nil, err
	}

	err = setNamespaces(r.Factory, infos, r.Namespace, r.EnforceNamespace)
	if err != nil {
		return nil, err
	}
	return infos, nil
}

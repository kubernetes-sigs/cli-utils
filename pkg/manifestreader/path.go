// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"k8s.io/cli-runtime/pkg/resource"
)

// PathManifestReader reads manifests from the provided path
// and returns them as Info objects. The returned Infos will not have
// client or mapping set.
type PathManifestReader struct {
	Path string

	ReaderOptions
}

// Read reads the manifests and returns them as Info objects.
func (p *PathManifestReader) Read() ([]*resource.Info, error) {
	validator, err := p.Factory.Validator(p.Validate)
	if err != nil {
		return nil, err
	}

	fileNameOptions := &resource.FilenameOptions{
		Filenames: []string{p.Path},
		Recursive: true,
	}

	enforceNamespace := false
	result := p.Factory.NewBuilder().
		Local().
		Unstructured().
		Schema(validator).
		ContinueOnError().
		FilenameParam(enforceNamespace, fileNameOptions).
		Flatten().
		Do()

	if err := result.Err(); err != nil {
		return nil, err
	}
	infos, err := result.Infos()
	if err != nil {
		return nil, err
	}
	infos = filterLocalConfig(infos)

	err = setNamespaces(p.Factory, infos, p.Namespace, p.EnforceNamespace)
	if err != nil {
		return nil, err
	}
	return infos, nil
}

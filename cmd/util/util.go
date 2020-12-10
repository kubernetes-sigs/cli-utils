// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"os"
	"path/filepath"

	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/setters2"
)

// KRMFileName returns the name of the KRM file. KRM file determines package
// boundaries and contains the openapi information for a package.
var KRMFileName = func() string {
	return "Krmfile"
}

// CheckForRequiredSetters takes the package path, checks if there is a KrmFile
// and checks if all the required setters are set
func CheckForRequiredSetters(path string) error {
	krmFileName := KRMFileName()
	krmFilePath := filepath.Join(path, krmFileName)
	_, err := os.Stat(krmFilePath)
	if err != nil {
		// if file is not readable or doesn't exist, exit without error
		// as there might be packages without KrmFile
		return nil
	}
	settersSchema, err := openapi.SchemaFromFile(krmFileName)
	if err != nil {
		return err
	}
	return setters2.CheckRequiredSettersSet(settersSchema)
}

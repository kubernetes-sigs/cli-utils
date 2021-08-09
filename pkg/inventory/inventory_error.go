// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Errors when applying inventory object templates.

package inventory

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const multipleInventoryErrorStr = `Package has multiple inventory object templates.

The package should have one and only one inventory object template.
`

const inventoryNamespaceInSet = `Inventory use namespace defined in package.

The inventory cannot use a namespace that is defined in the package.
`

type MultipleInventoryObjError struct {
	InventoryObjectTemplates []*unstructured.Unstructured
}

func (g MultipleInventoryObjError) Error() string {
	return multipleInventoryErrorStr
}

type InventoryNamespaceInSet struct {
	Namespace string
}

func (g InventoryNamespaceInSet) Error() string {
	return inventoryNamespaceInSet
}

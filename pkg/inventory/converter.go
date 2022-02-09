// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
)

type Converter interface {
	// GroupVersionKind returns the GVK supported by this Converter
	GroupVersionKind() schema.GroupVersionKind

	// To converts from an Unstructured of the supported GVK to an Inventory
	To(*unstructured.Unstructured) (*actuation.Inventory, error)

	// From converts from an Inventory to an Unstructured of the supported GVK
	From(*actuation.Inventory) (*unstructured.Unstructured, error)
}

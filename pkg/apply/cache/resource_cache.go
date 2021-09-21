// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ResourceCache stores unstructured resource objects in memory
type ResourceCache interface {
	// Put the resource into the cache, generating the ObjMetadata from the object.
	Put(obj *unstructured.Unstructured) error
	// Set the resource in the cache using the supplied key.
	Set(objMeta object.ObjMetadata, obj *unstructured.Unstructured)
	// Get the resource associated with the key from the cache.
	// Returns (nil, true) if not found in the cache.
	Get(objMeta object.ObjMetadata) (*unstructured.Unstructured, bool)
	// Remove the resource associated with the key from the cache.
	Remove(objMeta object.ObjMetadata)
	// Clear the cache.
	Clear()
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ResourceCacheMap stores unstructured resource objects in a map.
// ResourceCacheMap is NOT thread-safe.
type ResourceCacheMap struct {
	cache map[object.ObjMetadata]*unstructured.Unstructured
}

// NewResourceCacheMap returns a new empty ResourceCacheMap
func NewResourceCacheMap() *ResourceCacheMap {
	return &ResourceCacheMap{
		cache: make(map[object.ObjMetadata]*unstructured.Unstructured),
	}
}

// Load adds the resources into the cache, replacing any existing resource with
// the same ID. Returns an error if any resource is invalid.
func (rc *ResourceCacheMap) Load(values ...*unstructured.Unstructured) error {
	for _, value := range values {
		key, err := object.UnstructuredToObjMeta(value)
		if err != nil {
			return fmt.Errorf("failed to create resource cache key: %w", err)
		}
		rc.cache[key] = value
	}
	return nil
}

// Put adds the resource into the cache, replacing any existing resource with
// the same ID. Returns an error if resource is invalid.
func (rc *ResourceCacheMap) Put(value *unstructured.Unstructured) error {
	key, err := object.UnstructuredToObjMeta(value)
	if err != nil {
		return fmt.Errorf("failed to create resource cache key: %w", err)
	}
	rc.Set(key, value)
	return nil
}

// Set the resource in the cache using the supplied key, and replacing any
// existing resource with the same key.
func (rc *ResourceCacheMap) Set(key object.ObjMetadata, value *unstructured.Unstructured) {
	rc.cache[key] = value
}

// Get retrieves the resource associated with the key from the cache.
// Returns (nil, true) if not found in the cache.
func (rc *ResourceCacheMap) Get(key object.ObjMetadata) (*unstructured.Unstructured, bool) {
	obj, found := rc.cache[key]
	if klog.V(4).Enabled() {
		if found {
			klog.Infof("resource cache hit: %s", key)
		} else {
			klog.Infof("resource cache miss: %s", key)
		}
	}
	return obj, found
}

// Remove the resource associated with the key from the cache.
func (rc *ResourceCacheMap) Remove(key object.ObjMetadata) {
	delete(rc.cache, key)
}

// Clear the cache.
func (rc *ResourceCacheMap) Clear() {
	rc.cache = make(map[object.ObjMetadata]*unstructured.Unstructured)
}

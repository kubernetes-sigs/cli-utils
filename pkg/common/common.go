// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package common

const (
	// InventoryLabel is the label stored on the ConfigMap
	// inventory object. The value of the label is a unique
	// identifier (by default a UUID), representing the set of
	// objects applied at the same time as the inventory object.
	// This inventory object is used for pruning and deletion.
	InventoryLabel = "cli-utils.sigs.k8s.io/inventory-id"
	// InventoryHash defines an annotation which stores the hash of
	// the set of objects applied at the same time as the inventory
	// object. This annotation is set on the inventory object at the
	// time of the apply. The hash is computed from the sorted strings
	// of the applied object's metadata (ObjMetadata). The hash is
	// used as a suffix of the inventory object name. Example:
	//   inventory-1e5824fb
	InventoryHash = "cli-utils.sigs.k8s.io/inventory-hash"
	// Resource lifecycle annotation key for "on-remove" operations.
	OnRemoveAnnotation = "cli-utils.sigs.k8s.io/on-remove"
	// Resource lifecycle annotation value to prevent deletion.
	OnRemoveKeep = "keep"
)

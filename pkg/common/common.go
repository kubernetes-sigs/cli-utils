// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"
	"math/rand"
)

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
	// Maximum random number, non-inclusive, eight digits.
	maxRandInt = 100000000
)

// RandomStr returns an eight-digit (with leading zeros) string of a
// random number seeded with the parameter.
func RandomStr(seed int64) string {
	rand.Seed(seed)
	randomInt := rand.Intn(maxRandInt)
	return fmt.Sprintf("%08d", randomInt)
}

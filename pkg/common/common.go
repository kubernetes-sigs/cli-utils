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

var Strategies = []DryRunStrategy{DryRunClient, DryRunServer}

type DryRunStrategy int

const (
	// DryRunNone indicates the client will make all mutating calls
	DryRunNone DryRunStrategy = iota

	// DryRunClient, or client-side dry-run, indicates the client will prevent
	// making mutating calls such as CREATE, PATCH, and DELETE
	DryRunClient

	// DryRunServer, or server-side dry-run, indicates the client will send
	// mutating calls to the APIServer with the dry-run parameter to prevent
	// persisting changes.
	//
	// Note that clients sending server-side dry-run calls should verify that
	// the APIServer and the resource supports server-side dry-run, and otherwise
	// clients should fail early.
	//
	// If a client sends a server-side dry-run call to an APIServer that doesn't
	// support server-side dry-run, then the APIServer will persist changes inadvertently.
	DryRunServer
)

// ClientDryRun returns true if input drs is DryRunClient
func (drs DryRunStrategy) ClientDryRun() bool {
	return drs == DryRunClient
}

// ServerDryRun returns true if input drs is DryRunServer
func (drs DryRunStrategy) ServerDryRun() bool {
	return drs == DryRunServer
}

// ClientOrServerDryRun returns true if input drs is either client or server dry run
func (drs DryRunStrategy) ClientOrServerDryRun() bool {
	return drs == DryRunClient || drs == DryRunServer
}

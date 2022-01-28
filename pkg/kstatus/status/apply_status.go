// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

// ApplyStatus captures the apply status for a resource.
//go:generate stringer -type=ApplyStatus -linecomment
type ApplyStatus int

const (
	ApplyPending   ApplyStatus = iota // ApplyPending
	ApplySucceeded                    // ApplySucceeded
	ApplySkipped                      // ApplySkipped
	ApplyFailed                       // ApplyFailed
	PrunePending                      // PrunePending
	PruneSucceeded                    // PruneSucceeded
	PruneSkipped                      // PruneSkipped
	PruneFailed                       // PruneFailed
)

// ReconcileStatus captures the reconcile status for a resource.
//go:generate stringer -type=ReconcileStatus -linecomment
type ReconcileStatus int

const (
	ReconcilePending   ReconcileStatus = iota // ReconcilePending
	ReconcileSucceeded                        // ReconcileSucceeded
	ReconcileSkipped                          // ReconcileSkipped
	ReconcileTimeout                          // ReconcileTimeout
	ReconcileFailed                           // ReconcileFailed
)

// ApplyReconcileStatus captures the apply and reconcile
// status for a resource.
type ApplyReconcileStatus struct {
	ApplyStatus
	ReconcileStatus
}

// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/print/stats"
)

// ResultErrorFromStats takes a stats object and returns either a ResultError or
// nil depending on whether the stats reports that resources failed apply/prune/delete
// or reconciliation.
func ResultErrorFromStats(s stats.Stats) error {
	if s.FailedActuationSum() > 0 || s.FailedReconciliationSum() > 0 {
		return &ResultError{
			Stats: s,
		}
	}
	return nil
}

// ResultError is returned from printers when the apply/destroy operations completed, but one or
// more resources either failed apply/prune/delete, or failed to reconcile.
type ResultError struct {
	Stats stats.Stats
}

func (a *ResultError) Error() string {
	switch {
	case a.Stats.FailedActuationSum() > 0 && a.Stats.FailedReconciliationSum() > 0:
		return fmt.Sprintf("%d resources failed, %d resources failed to reconcile before timeout",
			a.Stats.FailedActuationSum(), a.Stats.FailedReconciliationSum())
	case a.Stats.FailedActuationSum() > 0:
		return fmt.Sprintf("%d resources failed", a.Stats.FailedActuationSum())
	case a.Stats.FailedReconciliationSum() > 0:
		return fmt.Sprintf("%d resources failed to reconcile before timeout",
			a.Stats.FailedReconciliationSum())
	default:
		// Should not happen as this error is only used when at least one resource
		// either failed to apply/prune/delete or reconcile.
		return "unknown error"
	}
}

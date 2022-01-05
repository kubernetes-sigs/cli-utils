// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stats

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/apply/event"
)

// Stats captures the summarized numbers from apply/prune/delete and
// reconciliation of resources.
type Stats struct {
	ApplyStats  ApplyStats
	PruneStats  PruneStats
	DeleteStats DeleteStats
	WaitStats   WaitStats
}

// FailedActuationSum returns the number of resources that failed actuation.
func (s *Stats) FailedActuationSum() int {
	return s.ApplyStats.Failed + s.PruneStats.Failed + s.DeleteStats.Failed
}

// FailedReconciliationSum returns the number of resources that failed reconciliation.
func (s *Stats) FailedReconciliationSum() int {
	return s.WaitStats.Failed + s.WaitStats.Timeout
}

// Handle updates the stats based on an event.
func (s *Stats) Handle(e event.Event) {
	switch e.Type {
	case event.ApplyType:
		if e.ApplyEvent.Error != nil {
			s.ApplyStats.IncFailed()
			return
		}
		s.ApplyStats.Inc(e.ApplyEvent.Operation)
	case event.PruneType:
		if e.PruneEvent.Error != nil {
			s.PruneStats.IncFailed()
			return
		}
		s.PruneStats.Inc(e.PruneEvent.Operation)
	case event.DeleteType:
		if e.DeleteEvent.Error != nil {
			s.DeleteStats.IncFailed()
			return
		}
		s.DeleteStats.Inc(e.DeleteEvent.Operation)
	case event.WaitType:
		s.WaitStats.Inc(e.WaitEvent.Operation)
	}
}

type ApplyStats struct {
	ServersideApplied int
	Created           int
	Unchanged         int
	Configured        int
	Failed            int
}

func (a *ApplyStats) Inc(op event.ApplyEventOperation) {
	switch op {
	case event.ApplyUnspecified:
	case event.ServersideApplied:
		a.ServersideApplied++
	case event.Created:
		a.Created++
	case event.Unchanged:
		a.Unchanged++
	case event.Configured:
		a.Configured++
	default:
		panic(fmt.Errorf("unknown apply operation %s", op.String()))
	}
}

func (a *ApplyStats) IncFailed() {
	a.Failed++
}

func (a *ApplyStats) Sum() int {
	return a.ServersideApplied + a.Configured + a.Unchanged + a.Created + a.Failed
}

type PruneStats struct {
	Pruned  int
	Skipped int
	Failed  int
}

func (p *PruneStats) Inc(op event.PruneEventOperation) {
	switch op {
	case event.PruneUnspecified:
	case event.Pruned:
		p.Pruned++
	case event.PruneSkipped:
		p.Skipped++
	}
}

func (p *PruneStats) IncFailed() {
	p.Failed++
}

type DeleteStats struct {
	Deleted int
	Skipped int
	Failed  int
}

func (d *DeleteStats) Inc(op event.DeleteEventOperation) {
	switch op {
	case event.DeleteUnspecified:
	case event.Deleted:
		d.Deleted++
	case event.DeleteSkipped:
		d.Skipped++
	}
}

func (d *DeleteStats) IncFailed() {
	d.Failed++
}

type WaitStats struct {
	Reconciled int
	Timeout    int
	Failed     int
	Skipped    int
}

func (w *WaitStats) Inc(op event.WaitEventOperation) {
	switch op {
	case event.Reconciled:
		w.Reconciled++
	case event.ReconcileSkipped:
		w.Skipped++
	case event.ReconcileTimeout:
		w.Timeout++
	case event.ReconcileFailed:
		w.Failed++
	}
}

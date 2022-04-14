// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stats

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/apply/event"
)

// Stats captures the summarized numbers from apply/prune/delete and
// reconciliation of resources. Each item in a stats list represents the stats
// from all the events in a single action group.
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
		s.ApplyStats.Inc(e.ApplyEvent.Status)
	case event.PruneType:
		s.PruneStats.Inc(e.PruneEvent.Status)
	case event.DeleteType:
		s.DeleteStats.Inc(e.DeleteEvent.Status)
	case event.WaitType:
		s.WaitStats.Inc(e.WaitEvent.Status)
	}
}

type ApplyStats struct {
	Successful int
	Skipped    int
	Failed     int
}

func (a *ApplyStats) Inc(op event.ApplyEventStatus) {
	switch op {
	case event.ApplySuccessful:
		a.Successful++
	case event.ApplySkipped:
		a.Skipped++
	case event.ApplyFailed:
		a.Failed++
	default:
		panic(fmt.Errorf("invalid apply status %s", op.String()))
	}
}

func (a *ApplyStats) IncFailed() {
	a.Failed++
}

func (a *ApplyStats) Sum() int {
	return a.Successful + a.Skipped + a.Failed
}

type PruneStats struct {
	Successful int
	Skipped    int
	Failed     int
}

func (p *PruneStats) Inc(op event.PruneEventStatus) {
	switch op {
	case event.PruneSuccessful:
		p.Successful++
	case event.PruneSkipped:
		p.Skipped++
	case event.PruneFailed:
		p.Failed++
	default:
		panic(fmt.Errorf("invalid prune status %s", op.String()))
	}
}

func (p *PruneStats) IncFailed() {
	p.Failed++
}

func (p *PruneStats) Sum() int {
	return p.Successful + p.Skipped + p.Failed
}

type DeleteStats struct {
	Successful int
	Skipped    int
	Failed     int
}

func (d *DeleteStats) Inc(op event.DeleteEventStatus) {
	switch op {
	case event.DeleteSuccessful:
		d.Successful++
	case event.DeleteSkipped:
		d.Skipped++
	case event.DeleteFailed:
		d.Failed++
	default:
		panic(fmt.Errorf("invalid delete status %s", op.String()))
	}
}

func (d *DeleteStats) IncFailed() {
	d.Failed++
}

func (d *DeleteStats) Sum() int {
	return d.Successful + d.Skipped + d.Failed
}

type WaitStats struct {
	Successful int
	Timeout    int
	Failed     int
	Skipped    int
}

func (w *WaitStats) Inc(status event.WaitEventStatus) {
	switch status {
	case event.ReconcilePending:
		// ignore - should be replaced by one of the others before the WaitTask exits
	case event.ReconcileSuccessful:
		w.Successful++
	case event.ReconcileSkipped:
		w.Skipped++
	case event.ReconcileTimeout:
		w.Timeout++
	case event.ReconcileFailed:
		w.Failed++
	default:
		panic(fmt.Errorf("invalid wait status %s", status.String()))
	}
}

func (w *WaitStats) Sum() int {
	return w.Successful + w.Skipped + w.Failed + w.Timeout
}

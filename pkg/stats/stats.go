// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stats

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/apply/event"
)

type Collector struct {
	Stats Stats
}

func NewCollector() *Collector {
	return &Collector{}
}

func (n *Collector) Collect(eventChannel <-chan event.Event) <-chan event.Event {
	outChannel := make(chan event.Event)
	var stats Stats
	go func() {
		defer close(outChannel)
		for ev := range eventChannel {
			stats.Handle(ev)
			outChannel <- ev
		}
	}()
	return outChannel
}

type Stats struct {
	ApplyStats  ApplyStats
	PruneStats  PruneStats
	DeleteStats DeleteStats
	WaitStats   WaitStats
}

func (s *Stats) FailedActuationSum() int {
	return s.ApplyStats.Failed + s.PruneStats.Failed + s.DeleteStats.Failed
}

func (s *Stats) FailedReconciliationSum() int {
	return s.WaitStats.Failed + s.WaitStats.Timeout
}

func (s *Stats) Handle(e event.Event) {
	switch e.Type {
	case event.ApplyType:
		if e.ApplyEvent.Error != nil {
			s.ApplyStats.incFailed()
			return
		}
		s.ApplyStats.inc(e.ApplyEvent.Operation)
	case event.PruneType:
		if e.PruneEvent.Error != nil {
			s.PruneStats.incFailed()
			return
		}
		s.PruneStats.inc(e.PruneEvent.Operation)
	case event.DeleteType:
		if e.DeleteEvent.Error != nil {
			s.DeleteStats.incFailed()
			return
		}
		s.DeleteStats.inc(e.DeleteEvent.Operation)
	case event.WaitType:
		s.WaitStats.inc(e.WaitEvent.Operation)
	}
}

type ApplyStats struct {
	ServersideApplied int
	Created           int
	Unchanged         int
	Configured        int
	Failed            int
}

func (a *ApplyStats) inc(op event.ApplyEventOperation) {
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

func (a *ApplyStats) incFailed() {
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

func (p *PruneStats) inc(op event.PruneEventOperation) {
	switch op {
	case event.Pruned:
		p.Pruned++
	case event.PruneSkipped:
		p.Skipped++
	}
}

func (p *PruneStats) incFailed() {
	p.Failed++
}

type DeleteStats struct {
	Deleted int
	Skipped int
	Failed  int
}

func (d *DeleteStats) inc(op event.DeleteEventOperation) {
	switch op {
	case event.Deleted:
		d.Deleted++
	case event.DeleteSkipped:
		d.Skipped++
	}
}

func (d *DeleteStats) incFailed() {
	d.Failed++
}

type WaitStats struct {
	Reconciled int
	Timeout    int
	Failed     int
	Skipped    int
}

func (w *WaitStats) inc(op event.WaitEventOperation) {
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

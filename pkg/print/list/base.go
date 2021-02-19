// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package list

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type Formatter interface {
	FormatApplyEvent(ae event.ApplyEvent, as *ApplyStats, c Collector) error
	FormatStatusEvent(se event.StatusEvent, sc Collector) error
	FormatPruneEvent(pe event.PruneEvent, ps *PruneStats) error
	FormatDeleteEvent(de event.DeleteEvent, ds *DeleteStats) error
	FormatErrorEvent(ee event.ErrorEvent) error
}

type FormatterFactory func(ioStreams genericclioptions.IOStreams,
	previewStrategy common.DryRunStrategy) Formatter

type BaseListPrinter struct {
	FormatterFactory FormatterFactory
	IOStreams        genericclioptions.IOStreams
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
	case event.ServersideApplied:
		a.ServersideApplied++
	case event.Created:
		a.Created++
	case event.Unchanged:
		a.Unchanged++
	case event.Configured:
		a.Configured++
	case event.Failed:
		a.Failed++
	default:
		panic(fmt.Errorf("unknown apply operation %s", op.String()))
	}
}

func (a *ApplyStats) Sum() int {
	return a.ServersideApplied + a.Configured + a.Unchanged + a.Created + a.Failed
}

type PruneStats struct {
	Pruned  int
	Skipped int
	Failed  int
}

func (p *PruneStats) incPruned() {
	p.Pruned++
}

func (p *PruneStats) incSkipped() {
	p.Skipped++
}

func (p *PruneStats) incFailed() {
	p.Failed++
}

type DeleteStats struct {
	Deleted int
	Skipped int
	Failed  int
}

func (d *DeleteStats) incDeleted() {
	d.Deleted++
}

func (d *DeleteStats) incSkipped() {
	d.Skipped++
}

func (d *DeleteStats) incFailed() {
	d.Failed++
}

type Collector interface {
	LatestStatus() map[object.ObjMetadata]event.StatusEvent
}

type StatusCollector struct {
	latestStatus map[object.ObjMetadata]event.StatusEvent
}

func (sc *StatusCollector) updateStatus(id object.ObjMetadata, se event.StatusEvent) {
	sc.latestStatus[id] = se
}

func (sc *StatusCollector) LatestStatus() map[object.ObjMetadata]event.StatusEvent {
	return sc.latestStatus
}

// Print outputs the events from the provided channel in a simple
// format on StdOut. As we support other printer implementations
// this should probably be an interface.
// This function will block until the channel is closed.
func (b *BaseListPrinter) Print(ch <-chan event.Event, previewStrategy common.DryRunStrategy) error {
	applyStats := &ApplyStats{}
	statusCollector := &StatusCollector{
		latestStatus: make(map[object.ObjMetadata]event.StatusEvent),
	}
	printStatus := false
	pruneStats := &PruneStats{}
	deleteStats := &DeleteStats{}
	formatter := b.FormatterFactory(b.IOStreams, previewStrategy)
	for e := range ch {
		switch e.Type {
		case event.ErrorType:
			_ = formatter.FormatErrorEvent(e.ErrorEvent)
			return e.ErrorEvent.Err
		case event.ApplyType:
			if e.ApplyEvent.Type == event.ApplyEventResourceUpdate {
				applyStats.inc(e.ApplyEvent.Operation)
			}
			if e.ApplyEvent.Type == event.ApplyEventCompleted {
				printStatus = true
			}
			if err := formatter.FormatApplyEvent(e.ApplyEvent, applyStats, statusCollector); err != nil {
				return err
			}
		case event.StatusType:
			if se := e.StatusEvent; se.Type == event.StatusEventResourceUpdate {
				statusCollector.updateStatus(e.StatusEvent.Resource.Identifier, e.StatusEvent)
				if printStatus {
					if err := formatter.FormatStatusEvent(e.StatusEvent, statusCollector); err != nil {
						return err
					}
				}
			}
		case event.PruneType:
			if e.PruneEvent.Type == event.PruneEventResourceUpdate {
				switch e.PruneEvent.Operation {
				case event.Pruned:
					pruneStats.incPruned()
				case event.PruneSkipped:
					pruneStats.incSkipped()
				}
			}
			if e.PruneEvent.Type == event.PruneEventFailed {
				pruneStats.incFailed()
			}
			if err := formatter.FormatPruneEvent(e.PruneEvent, pruneStats); err != nil {
				return err
			}
		case event.DeleteType:
			if e.DeleteEvent.Type == event.DeleteEventResourceUpdate {
				switch e.DeleteEvent.Operation {
				case event.Deleted:
					deleteStats.incDeleted()
				case event.DeleteSkipped:
					deleteStats.incSkipped()
				}
			}
			if e.DeleteEvent.Type == event.DeleteEventFailed {
				deleteStats.incFailed()
			}
			if err := formatter.FormatDeleteEvent(e.DeleteEvent, deleteStats); err != nil {
				return err
			}
		}
	}
	failedSum := applyStats.Failed + pruneStats.Failed + deleteStats.Failed
	if failedSum > 0 {
		return fmt.Errorf("%d resources failed", failedSum)
	}
	return nil
}

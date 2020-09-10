// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package list

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type Formatter interface {
	FormatApplyEvent(ae event.ApplyEvent, as *ApplyStats, c Collector) error
	FormatStatusEvent(se pollevent.Event, sc Collector) error
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
	default:
		panic(fmt.Errorf("unknown apply operation %s", op.String()))
	}
}

func (a *ApplyStats) Sum() int {
	return a.ServersideApplied + a.Configured + a.Unchanged + a.Created
}

type PruneStats struct {
	Pruned  int
	Skipped int
}

func (p *PruneStats) incPruned() {
	p.Pruned++
}

func (p *PruneStats) incSkipped() {
	p.Skipped++
}

type DeleteStats struct {
	Deleted int
	Skipped int
}

func (d *DeleteStats) incDeleted() {
	d.Deleted++
}

func (d *DeleteStats) incSkipped() {
	d.Skipped++
}

type Collector interface {
	LatestStatus() map[object.ObjMetadata]pollevent.Event
}

type StatusCollector struct {
	latestStatus map[object.ObjMetadata]pollevent.Event
}

func (sc *StatusCollector) updateStatus(id object.ObjMetadata, se pollevent.Event) {
	sc.latestStatus[id] = se
}

func (sc *StatusCollector) LatestStatus() map[object.ObjMetadata]pollevent.Event {
	return sc.latestStatus
}

// Print outputs the events from the provided channel in a simple
// format on StdOut. As we support other printer implementations
// this should probably be an interface.
// This function will block until the channel is closed.
func (b *BaseListPrinter) Print(ch <-chan event.Event, previewStrategy common.DryRunStrategy) error {
	applyStats := &ApplyStats{}
	statusCollector := &StatusCollector{
		latestStatus: make(map[object.ObjMetadata]pollevent.Event),
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
			switch se := e.StatusEvent; se.EventType {
			case pollevent.ResourceUpdateEvent:
				statusCollector.updateStatus(e.StatusEvent.Resource.Identifier, e.StatusEvent)
				if printStatus {
					if err := formatter.FormatStatusEvent(e.StatusEvent, statusCollector); err != nil {
						return err
					}
				}
			case pollevent.ErrorEvent:
				if err := formatter.FormatStatusEvent(e.StatusEvent, statusCollector); err != nil {
					return err
				}
			case pollevent.CompletedEvent:
				printStatus = false
				if err := formatter.FormatStatusEvent(e.StatusEvent, statusCollector); err != nil {
					return err
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
			if err := formatter.FormatDeleteEvent(e.DeleteEvent, deleteStats); err != nil {
				return err
			}
		}
	}
	return nil
}

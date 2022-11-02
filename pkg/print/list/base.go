// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package list

import (
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	printcommon "sigs.k8s.io/cli-utils/pkg/print/common"
	"sigs.k8s.io/cli-utils/pkg/print/stats"
)

type Formatter interface {
	FormatValidationEvent(ve event.ValidationEvent) error
	FormatApplyEvent(ae event.ApplyEvent) error
	FormatStatusEvent(se event.StatusEvent) error
	FormatPruneEvent(pe event.PruneEvent) error
	FormatDeleteEvent(de event.DeleteEvent) error
	FormatWaitEvent(we event.WaitEvent) error
	FormatErrorEvent(ee event.ErrorEvent) error
	FormatActionGroupEvent(
		age event.ActionGroupEvent,
		ags []event.ActionGroup,
		s stats.Stats,
		c Collector,
	) error
	FormatSummary(s stats.Stats) error
}

type FormatterFactory func(previewStrategy common.DryRunStrategy) Formatter

type BaseListPrinter struct {
	FormatterFactory FormatterFactory
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
//
//nolint:gocyclo
func (b *BaseListPrinter) Print(ch <-chan event.Event, previewStrategy common.DryRunStrategy, printStatus bool) error {
	var actionGroups []event.ActionGroup
	var statsCollector stats.Stats
	statusCollector := &StatusCollector{
		latestStatus: make(map[object.ObjMetadata]event.StatusEvent),
	}
	formatter := b.FormatterFactory(previewStrategy)
	for e := range ch {
		statsCollector.Handle(e)
		switch e.Type {
		case event.InitType:
			actionGroups = e.InitEvent.ActionGroups
		case event.ErrorType:
			_ = formatter.FormatErrorEvent(e.ErrorEvent)
			return e.ErrorEvent.Err
		case event.ValidationType:
			if err := formatter.FormatValidationEvent(e.ValidationEvent); err != nil {
				return err
			}
		case event.ApplyType:
			if err := formatter.FormatApplyEvent(e.ApplyEvent); err != nil {
				return err
			}
		case event.StatusType:
			statusCollector.updateStatus(e.StatusEvent.Identifier, e.StatusEvent)
			if printStatus {
				if err := formatter.FormatStatusEvent(e.StatusEvent); err != nil {
					return err
				}
			}
		case event.PruneType:
			if err := formatter.FormatPruneEvent(e.PruneEvent); err != nil {
				return err
			}
		case event.DeleteType:
			if err := formatter.FormatDeleteEvent(e.DeleteEvent); err != nil {
				return err
			}
		case event.WaitType:
			if err := formatter.FormatWaitEvent(e.WaitEvent); err != nil {
				return err
			}
		case event.ActionGroupType:
			if err := formatter.FormatActionGroupEvent(
				e.ActionGroupEvent,
				actionGroups,
				statsCollector,
				statusCollector,
			); err != nil {
				return err
			}
		}
	}

	if err := formatter.FormatSummary(statsCollector); err != nil {
		return err
	}
	return printcommon.ResultErrorFromStats(statsCollector)
}

// IsLastActionGroup returns true if the passed ActionGroupEvent is the
// last of its type in the slice of ActionGroup; false otherwise. For example,
// this function will determine if an ApplyAction is the last ApplyAction in
// the initialized task queue. This functionality is current used to determine
// when to print stats.
func IsLastActionGroup(age event.ActionGroupEvent, ags []event.ActionGroup) bool {
	var found bool
	var action event.ResourceAction
	for _, ag := range ags {
		if found && (action == ag.Action) {
			return false
		}
		if age.GroupName == ag.Name {
			found = true
			action = age.Action
		}
	}
	return true
}

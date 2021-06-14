// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type ExpEvent struct {
	EventType event.Type

	ActionGroupEvent *ExpActionGroupEvent
	ApplyEvent       *ExpApplyEvent
	StatusEvent      *ExpStatusEvent
	PruneEvent       *ExpPruneEvent
	DeleteEvent      *ExpDeleteEvent
}

type ExpActionGroupEvent struct {
	Name   string
	Action event.ResourceAction
}

type ExpApplyEvent struct {
	Operation  event.ApplyEventOperation
	Identifier object.ObjMetadata
	Error      error
}

type ExpStatusEvent struct {
	Identifier object.ObjMetadata
	Status     status.Status
	Error      error
}

type ExpPruneEvent struct {
	Operation  event.PruneEventOperation
	Identifier object.ObjMetadata
	Error      error
}

type ExpDeleteEvent struct {
	Operation  event.DeleteEventOperation
	Identifier object.ObjMetadata
	Error      error
}

func VerifyEvents(expEvents []ExpEvent, events []event.Event) error {
	if len(expEvents) == 0 && len(events) == 0 {
		return nil
	}
	expEventIndex := 0
	for i := range events {
		e := events[i]
		ee := expEvents[expEventIndex]
		if isMatch(ee, e) {
			expEventIndex += 1
			if expEventIndex >= len(expEvents) {
				return nil
			}
		}
	}
	return fmt.Errorf("event %s not found", expEvents[expEventIndex].EventType)
}

var nilIdentifier = object.ObjMetadata{}

// nolint:gocyclo
// TODO(mortent): This function is pretty complex and with quite a bit of
// duplication. We should see if there is a better way to provide a flexible
// way to verify that we go the expected events.
func isMatch(ee ExpEvent, e event.Event) bool {
	if ee.EventType != e.Type {
		return false
	}

	// nolint:gocritic
	switch e.Type {
	case event.ActionGroupType:
		agee := ee.ActionGroupEvent

		if agee == nil {
			return true
		}

		age := e.ActionGroupEvent

		if agee.Name != age.GroupName {
			return false
		}

		if agee.Action != age.Action {
			return false
		}
	case event.ApplyType:
		aee := ee.ApplyEvent
		// If no more information is specified, we consider it a match.
		if aee == nil {
			return true
		}
		ae := e.ApplyEvent

		if aee.Identifier != nilIdentifier {
			if aee.Identifier != ae.Identifier {
				return false
			}
		}

		if aee.Operation != ae.Operation {
			return false
		}

		if aee.Error != nil {
			return ae.Error != nil
		}
		return ae.Error == nil

	case event.StatusType:
		see := ee.StatusEvent
		if see == nil {
			return true
		}
		se := e.StatusEvent

		if see.Identifier != se.Identifier {
			return false
		}

		if see.Status != se.PollResourceInfo.Status {
			return false
		}

		if see.Error != nil {
			return se.Error != nil
		}
		return se.Error == nil

	case event.PruneType:
		pee := ee.PruneEvent
		if pee == nil {
			return true
		}
		pe := e.PruneEvent

		if pee.Identifier != nilIdentifier {
			if pee.Identifier != pe.Identifier {
				return false
			}
		}

		if pee.Operation != pe.Operation {
			return false
		}

		if pee.Error != nil {
			return pe.Error != nil
		}
		return pe.Error == nil

	case event.DeleteType:
		dee := ee.DeleteEvent
		if dee == nil {
			return true
		}
		de := e.DeleteEvent

		if dee.Identifier != nilIdentifier {
			if dee.Identifier != de.Identifier {
				return false
			}
		}

		if dee.Operation != de.Operation {
			return false
		}

		if dee.Error != nil {
			return de.Error != nil
		}
		return de.Error == nil
	}
	return true
}

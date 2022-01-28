// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type ExpEvent struct {
	EventType event.Type

	InitEvent        *ExpInitEvent
	ErrorEvent       *ExpErrorEvent
	ActionGroupEvent *ExpActionGroupEvent
	ApplyEvent       *ExpApplyEvent
	StatusEvent      *ExpStatusEvent
	PruneEvent       *ExpPruneEvent
	DeleteEvent      *ExpDeleteEvent
	WaitEvent        *ExpWaitEvent
	ValidationEvent  *ExpValidationEvent
}

type ExpInitEvent struct {
	// TODO: enable if we want to more thuroughly test InitEvents
	// ActionGroups []event.ActionGroup
}

type ExpErrorEvent struct {
	Err error
}

type ExpActionGroupEvent struct {
	GroupName string
	Action    event.ResourceAction
	Type      event.ActionGroupEventType
}

type ExpApplyEvent struct {
	GroupName  string
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
	GroupName  string
	Operation  event.PruneEventOperation
	Identifier object.ObjMetadata
	Error      error
}

type ExpDeleteEvent struct {
	GroupName  string
	Operation  event.DeleteEventOperation
	Identifier object.ObjMetadata
	Error      error
}

type ExpWaitEvent struct {
	GroupName  string
	Operation  event.WaitEventOperation
	Identifier object.ObjMetadata
}

type ExpValidationEvent struct {
	Identifiers object.ObjMetadataSet
	Error       error
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
	case event.ErrorType:
		a := ee.ErrorEvent

		if a == nil {
			return true
		}

		b := e.ErrorEvent

		if a.Err != nil {
			if !cmp.Equal(a.Err, b.Err, cmpopts.EquateErrors()) {
				return false
			}
		}
		return true

	case event.ActionGroupType:
		agee := ee.ActionGroupEvent

		if agee == nil {
			return true
		}

		age := e.ActionGroupEvent

		if agee.GroupName != age.GroupName {
			return false
		}

		if agee.Action != age.Action {
			return false
		}

		if agee.Type != age.Type {
			return false
		}
		return true

	case event.ApplyType:
		aee := ee.ApplyEvent
		// If no more information is specified, we consider it a match.
		if aee == nil {
			return true
		}
		ae := e.ApplyEvent

		if aee.Identifier != object.NilObjMetadata {
			if aee.Identifier != ae.Identifier {
				return false
			}
		}

		if aee.GroupName != "" {
			if aee.GroupName != ae.GroupName {
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

		if pee.Identifier != object.NilObjMetadata {
			if pee.Identifier != pe.Identifier {
				return false
			}
		}

		if pee.GroupName != "" {
			if pee.GroupName != pe.GroupName {
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

		if dee.Identifier != object.NilObjMetadata {
			if dee.Identifier != de.Identifier {
				return false
			}
		}

		if dee.GroupName != "" {
			if dee.GroupName != de.GroupName {
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

	case event.WaitType:
		wee := ee.WaitEvent
		if wee == nil {
			return true
		}
		we := e.WaitEvent

		if wee.Identifier != object.NilObjMetadata {
			if wee.Identifier != we.Identifier {
				return false
			}
		}

		if wee.GroupName != "" {
			if wee.GroupName != we.GroupName {
				return false
			}
		}

		if wee.Operation != we.Operation {
			return false
		}
		return true

	case event.ValidationType:
		vee := ee.ValidationEvent
		if vee == nil {
			return true
		}
		ve := e.ValidationEvent

		if vee.Identifiers != nil {
			if !vee.Identifiers.Equal(ve.Identifiers) {
				return false
			}
		}

		if vee.Error != nil {
			return ve.Error != nil
		}
		return ve.Error == nil

	default:
		return true
	}
}

func EventsToExpEvents(events []event.Event) []ExpEvent {
	result := make([]ExpEvent, 0, len(events))
	for _, event := range events {
		result = append(result, EventToExpEvent(event))
	}
	return result
}

func EventToExpEvent(e event.Event) ExpEvent {
	switch e.Type {
	case event.InitType:
		return ExpEvent{
			EventType: event.InitType,
			InitEvent: &ExpInitEvent{
				// TODO: enable if we want to more thuroughly test InitEvents
				// ActionGroups: e.InitEvent.ActionGroups,
			},
		}

	case event.ErrorType:
		return ExpEvent{
			EventType: event.ErrorType,
			ErrorEvent: &ExpErrorEvent{
				Err: e.ErrorEvent.Err,
			},
		}

	case event.ActionGroupType:
		return ExpEvent{
			EventType: event.ActionGroupType,
			ActionGroupEvent: &ExpActionGroupEvent{
				GroupName: e.ActionGroupEvent.GroupName,
				Action:    e.ActionGroupEvent.Action,
				Type:      e.ActionGroupEvent.Type,
			},
		}

	case event.ApplyType:
		return ExpEvent{
			EventType: event.ApplyType,
			ApplyEvent: &ExpApplyEvent{
				GroupName:  e.ApplyEvent.GroupName,
				Identifier: e.ApplyEvent.Identifier,
				Operation:  e.ApplyEvent.Operation,
				Error:      e.ApplyEvent.Error,
			},
		}

	case event.StatusType:
		return ExpEvent{
			EventType: event.StatusType,
			StatusEvent: &ExpStatusEvent{
				Identifier: e.StatusEvent.Identifier,
				Status:     e.StatusEvent.PollResourceInfo.Status,
				Error:      e.StatusEvent.Error,
			},
		}

	case event.PruneType:
		return ExpEvent{
			EventType: event.PruneType,
			PruneEvent: &ExpPruneEvent{
				GroupName:  e.PruneEvent.GroupName,
				Identifier: e.PruneEvent.Identifier,
				Operation:  e.PruneEvent.Operation,
				Error:      e.PruneEvent.Error,
			},
		}

	case event.DeleteType:
		return ExpEvent{
			EventType: event.DeleteType,
			DeleteEvent: &ExpDeleteEvent{
				GroupName:  e.DeleteEvent.GroupName,
				Identifier: e.DeleteEvent.Identifier,
				Operation:  e.DeleteEvent.Operation,
				Error:      e.DeleteEvent.Error,
			},
		}

	case event.WaitType:
		return ExpEvent{
			EventType: event.WaitType,
			WaitEvent: &ExpWaitEvent{
				GroupName:  e.WaitEvent.GroupName,
				Identifier: e.WaitEvent.Identifier,
				Operation:  e.WaitEvent.Operation,
			},
		}

	case event.ValidationType:
		return ExpEvent{
			EventType: event.ValidationType,
			ValidationEvent: &ExpValidationEvent{
				Identifiers: e.ValidationEvent.Identifiers,
				Error:       e.ValidationEvent.Error,
			},
		}
	}
	return ExpEvent{}
}

func RemoveEqualEvents(in []ExpEvent, expected ExpEvent) ([]ExpEvent, int) {
	matches := 0
	for i := 0; i < len(in); i++ {
		if cmp.Equal(in[i], expected, cmpopts.EquateErrors()) {
			// remove event at index i
			in = append(in[:i], in[i+1:]...)
			matches++
			i--
		}
	}
	return in, matches
}

// GroupedEventsByID implements sort.Interface for []ExpEvent based on
// the serialized ObjMetadata of Apply, Prune, and Delete events within the same
// task group.
// This makes testing events easier, because apply/prune/delete order is
// non-deterministic within each task group.
// This is only needed if you expect to have multiple apply/prune/delete events
// in the same task group.
type GroupedEventsByID []ExpEvent

func (ape GroupedEventsByID) Len() int      { return len(ape) }
func (ape GroupedEventsByID) Swap(i, j int) { ape[i], ape[j] = ape[j], ape[i] }
func (ape GroupedEventsByID) Less(i, j int) bool {
	if ape[i].EventType != ape[j].EventType {
		// don't change order if not the same type
		return false
	}
	switch ape[i].EventType {
	case event.ApplyType:
		if ape[i].ApplyEvent.GroupName != ape[j].ApplyEvent.GroupName {
			// don't change order if not the same task group
			return false
		}
		return ape[i].ApplyEvent.Identifier.String() < ape[j].ApplyEvent.Identifier.String()
	case event.PruneType:
		if ape[i].PruneEvent.GroupName != ape[j].PruneEvent.GroupName {
			// don't change order if not the same task group
			return false
		}
		return ape[i].PruneEvent.Identifier.String() < ape[j].PruneEvent.Identifier.String()
	case event.DeleteType:
		if ape[i].DeleteEvent.GroupName != ape[j].DeleteEvent.GroupName {
			// don't change order if not the same task group
			return false
		}
		return ape[i].DeleteEvent.Identifier.String() < ape[j].DeleteEvent.Identifier.String()
	case event.WaitType:
		if ape[i].WaitEvent.GroupName != ape[j].WaitEvent.GroupName {
			// don't change order if not the same task group
			return false
		}
		if ape[i].WaitEvent.Operation != ape[j].WaitEvent.Operation {
			// don't change order if not the same operation
			return false
		}
		if ape[i].WaitEvent.Operation != event.Reconciled {
			// pending, skipped, and timeout operations are predictably ordered
			// using the order in WaitTask.Ids.
			// So we only need to sort Reconciled events, which occur in the
			// order the Waitask receives StatusEvents with Current/NotFound.
			return false
		}
		return ape[i].WaitEvent.Identifier.String() < ape[j].WaitEvent.Identifier.String()
	case event.ValidationType:
		return ape[i].ValidationEvent.Identifiers.Hash() < ape[j].ValidationEvent.Identifiers.Hash()
	default:
		// don't change order if not ApplyType, PruneType, or DeleteType
		return false
	}
}

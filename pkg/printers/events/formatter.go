// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/print/list"
	"sigs.k8s.io/cli-utils/pkg/print/stats"
)

func NewFormatter(ioStreams genericiooptions.IOStreams,
	_ common.DryRunStrategy) list.Formatter {
	return &formatter{
		ioStreams: ioStreams,
	}
}

type formatter struct {
	ioStreams genericiooptions.IOStreams
}

func (ef *formatter) FormatValidationEvent(ve event.ValidationEvent) error {
	// unwrap validation errors
	err := ve.Error
	if vErr, ok := err.(*validation.Error); ok {
		err = vErr.Unwrap()
	}

	switch {
	case len(ve.Identifiers) == 0:
		// no objects, invalid event
		return fmt.Errorf("invalid validation event: no identifiers: %w", err)
	case len(ve.Identifiers) == 1:
		// only 1 object, unwrap for similarity with status event
		id := ve.Identifiers[0]
		ef.print("Invalid object (%s): %v",
			resourceIDToString(id.GroupKind, id.Name), err.Error())
	default:
		// more than 1 object, wrap list in brackets
		var sb strings.Builder
		id := ve.Identifiers[0]
		_, _ = fmt.Fprintf(&sb, "Invalid objects (%s", resourceIDToString(id.GroupKind, id.Name))
		for _, id := range ve.Identifiers[1:] {
			_, _ = fmt.Fprintf(&sb, ", %s", resourceIDToString(id.GroupKind, id.Name))
		}
		_, _ = fmt.Fprintf(&sb, "): %v", err)
		ef.print(sb.String())
	}
	return nil
}

func (ef *formatter) FormatApplyEvent(e event.ApplyEvent) error {
	gk := e.Identifier.GroupKind
	name := e.Identifier.Name
	if e.Error != nil {
		ef.print("%s apply %s: %s", resourceIDToString(gk, name),
			strings.ToLower(e.Status.String()), e.Error.Error())
	} else {
		ef.print("%s apply %s", resourceIDToString(gk, name),
			strings.ToLower(e.Status.String()))
	}
	return nil
}

func (ef *formatter) FormatStatusEvent(se event.StatusEvent) error {
	id := se.Identifier
	ef.printResourceStatus(id, se)
	return nil
}

func (ef *formatter) FormatPruneEvent(e event.PruneEvent) error {
	gk := e.Identifier.GroupKind
	name := e.Identifier.Name
	if e.Error != nil {
		ef.print("%s prune %s: %s", resourceIDToString(gk, name),
			strings.ToLower(e.Status.String()), e.Error.Error())
	} else {
		ef.print("%s prune %s", resourceIDToString(gk, name),
			strings.ToLower(e.Status.String()))
	}
	return nil
}

func (ef *formatter) FormatDeleteEvent(e event.DeleteEvent) error {
	gk := e.Identifier.GroupKind
	name := e.Identifier.Name
	if e.Error != nil {
		ef.print("%s delete %s: %s", resourceIDToString(gk, name),
			strings.ToLower(e.Status.String()), e.Error.Error())
	} else {
		ef.print("%s delete %s", resourceIDToString(gk, name),
			strings.ToLower(e.Status.String()))
	}
	return nil
}

func (ef *formatter) FormatWaitEvent(e event.WaitEvent) error {
	gk := e.Identifier.GroupKind
	name := e.Identifier.Name
	ef.print("%s reconcile %s", resourceIDToString(gk, name),
		strings.ToLower(e.Status.String()))
	return nil
}

func (ef *formatter) FormatErrorEvent(_ event.ErrorEvent) error {
	return nil
}

func (ef *formatter) FormatActionGroupEvent(
	age event.ActionGroupEvent,
	ags []event.ActionGroup,
	s stats.Stats,
	_ list.Collector,
) error {
	switch age.Action {
	case event.ApplyAction:
		ef.print("apply phase %s", strings.ToLower(age.Status.String()))
	case event.PruneAction:
		ef.print("prune phase %s", strings.ToLower(age.Status.String()))
	case event.DeleteAction:
		ef.print("delete phase %s", strings.ToLower(age.Status.String()))
	case event.WaitAction:
		ef.print("reconcile phase %s", strings.ToLower(age.Status.String()))
	case event.InventoryAction:
		ef.print("inventory update %s", strings.ToLower(age.Status.String()))
	default:
		return fmt.Errorf("invalid action group action: %+v", age)
	}
	return nil
}

func (ef *formatter) FormatSummary(s stats.Stats) error {
	if s.ApplyStats != (stats.ApplyStats{}) {
		as := s.ApplyStats
		ef.print("apply result: %d attempted, %d successful, %d skipped, %d failed",
			as.Sum(), as.Successful, as.Skipped, as.Failed)
	}
	if s.PruneStats != (stats.PruneStats{}) {
		ps := s.PruneStats
		ef.print("prune result: %d attempted, %d successful, %d skipped, %d failed",
			ps.Sum(), ps.Successful, ps.Skipped, ps.Failed)
	}
	if s.DeleteStats != (stats.DeleteStats{}) {
		ds := s.DeleteStats
		ef.print("delete result: %d attempted, %d successful, %d skipped, %d failed",
			ds.Sum(), ds.Successful, ds.Skipped, ds.Failed)
	}
	if s.WaitStats != (stats.WaitStats{}) {
		ws := s.WaitStats
		ef.print("reconcile result: %d attempted, %d successful, %d skipped, %d failed, %d timed out",
			ws.Sum(), ws.Successful, ws.Skipped, ws.Failed, ws.Timeout)
	}
	return nil
}

func (ef *formatter) printResourceStatus(id object.ObjMetadata, se event.StatusEvent) {
	ef.print("%s is %s: %s", resourceIDToString(id.GroupKind, id.Name),
		se.PollResourceInfo.Status.String(), se.PollResourceInfo.Message)
}

func (ef *formatter) print(format string, a ...any) {
	_, _ = fmt.Fprintf(ef.ioStreams.Out, format+"\n", a...)
}

// resourceIDToString returns the string representation of a GroupKind and a resource name.
func resourceIDToString(gk schema.GroupKind, name string) string {
	return fmt.Sprintf("%s/%s", strings.ToLower(gk.String()), name)
}

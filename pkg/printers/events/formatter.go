// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/print/list"
	"sigs.k8s.io/cli-utils/pkg/print/stats"
)

func NewFormatter(ioStreams genericclioptions.IOStreams,
	_ common.DryRunStrategy) list.Formatter {
	return &formatter{
		ioStreams: ioStreams,
	}
}

type formatter struct {
	ioStreams genericclioptions.IOStreams
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

func (ef *formatter) FormatApplyEvent(ae event.ApplyEvent) error {
	gk := ae.Identifier.GroupKind
	name := ae.Identifier.Name
	if ae.Error != nil {
		ef.print("%s apply failed: %s", resourceIDToString(gk, name),
			ae.Error.Error())
	} else {
		ef.print("%s %s", resourceIDToString(gk, name),
			strings.ToLower(ae.Operation.String()))
	}
	return nil
}

func (ef *formatter) FormatStatusEvent(se event.StatusEvent) error {
	id := se.Identifier
	ef.printResourceStatus(id, se)
	return nil
}

func (ef *formatter) FormatPruneEvent(pe event.PruneEvent) error {
	gk := pe.Identifier.GroupKind
	if pe.Error != nil {
		ef.print("%s prune failed: %s", resourceIDToString(gk, pe.Identifier.Name),
			pe.Error.Error())
		return nil
	}

	switch pe.Operation {
	case event.Pruned:
		ef.print("%s pruned", resourceIDToString(gk, pe.Identifier.Name))
	case event.PruneSkipped:
		ef.print("%s prune skipped", resourceIDToString(gk, pe.Identifier.Name))
	}
	return nil
}

func (ef *formatter) FormatDeleteEvent(de event.DeleteEvent) error {
	gk := de.Identifier.GroupKind
	name := de.Identifier.Name

	if de.Error != nil {
		ef.print("%s deletion failed: %s", resourceIDToString(gk, name),
			de.Error.Error())
		return nil
	}

	switch de.Operation {
	case event.Deleted:
		ef.print("%s deleted", resourceIDToString(gk, name))
	case event.DeleteSkipped:
		ef.print("%s delete skipped", resourceIDToString(gk, name))
	}
	return nil
}

func (ef *formatter) FormatWaitEvent(we event.WaitEvent) error {
	gk := we.Identifier.GroupKind
	name := we.Identifier.Name

	switch we.Operation {
	case event.ReconcilePending:
		ef.print("%s reconcile pending", resourceIDToString(gk, name))
	case event.Reconciled:
		ef.print("%s reconciled", resourceIDToString(gk, name))
	case event.ReconcileSkipped:
		ef.print("%s reconcile skipped", resourceIDToString(gk, name))
	case event.ReconcileTimeout:
		ef.print("%s reconcile timeout", resourceIDToString(gk, name))
	case event.ReconcileFailed:
		ef.print("%s reconcile failed", resourceIDToString(gk, name))
	}
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
	if age.Action == event.ApplyAction &&
		age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		as := s.ApplyStats
		output := fmt.Sprintf("%d resource(s) applied. %d created, %d unchanged, %d configured, %d failed",
			as.Sum(), as.Created, as.Unchanged, as.Configured, as.Failed)
		// Only print information about serverside apply if some of the
		// resources actually were applied serverside.
		if as.ServersideApplied > 0 {
			output += fmt.Sprintf(", %d serverside applied", as.ServersideApplied)
		}
		ef.print(output)
	}

	if age.Action == event.PruneAction &&
		age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		ps := s.PruneStats
		ef.print("%d resource(s) pruned, %d skipped, %d failed to prune", ps.Pruned, ps.Skipped, ps.Failed)
	}

	if age.Action == event.DeleteAction &&
		age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		ds := s.DeleteStats
		ef.print("%d resource(s) deleted, %d skipped, %d failed to delete", ds.Deleted, ds.Skipped, ds.Failed)
	}

	if age.Action == event.WaitAction &&
		age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		ws := s.WaitStats
		ef.print("%d resource(s) reconciled, %d skipped, %d failed to reconcile, %d timed out", ws.Reconciled,
			ws.Skipped, ws.Failed, ws.Timeout)
	}
	return nil
}

func (ef *formatter) printResourceStatus(id object.ObjMetadata, se event.StatusEvent) {
	ef.print("%s is %s: %s", resourceIDToString(id.GroupKind, id.Name),
		se.PollResourceInfo.Status.String(), se.PollResourceInfo.Message)
}

func (ef *formatter) print(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(ef.ioStreams.Out, format+"\n", a...)
}

// resourceIDToString returns the string representation of a GroupKind and a resource name.
func resourceIDToString(gk schema.GroupKind, name string) string {
	return fmt.Sprintf("%s/%s", strings.ToLower(gk.String()), name)
}

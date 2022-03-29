// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"fmt"
	"time"

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

func (jf *formatter) FormatValidationEvent(ve event.ValidationEvent) error {
	// unwrap validation errors
	err := ve.Error
	if vErr, ok := err.(*validation.Error); ok {
		err = vErr.Unwrap()
	}
	if len(ve.Identifiers) == 0 {
		// no objects, invalid event
		return fmt.Errorf("invalid validation event: no identifiers: %w", err)
	}
	objects := make([]interface{}, len(ve.Identifiers))
	for i, id := range ve.Identifiers {
		objects[i] = jf.baseResourceEvent(id)
	}
	return jf.printEvent("validation", "validation", map[string]interface{}{
		"objects": objects,
		"error":   err.Error(),
	})
}

func (jf *formatter) FormatApplyEvent(ae event.ApplyEvent) error {
	eventInfo := jf.baseResourceEvent(ae.Identifier)
	if ae.Error != nil {
		eventInfo["error"] = ae.Error.Error()
		// skipped apply sets both error and operation
		if ae.Operation != event.Unchanged {
			return jf.printEvent("apply", "resourceFailed", eventInfo)
		}
	}
	eventInfo["operation"] = ae.Operation.String()
	return jf.printEvent("apply", "resourceApplied", eventInfo)
}

func (jf *formatter) FormatStatusEvent(se event.StatusEvent) error {
	return jf.printResourceStatus(se)
}

func (jf *formatter) printResourceStatus(se event.StatusEvent) error {
	eventInfo := jf.baseResourceEvent(se.Identifier)
	eventInfo["status"] = se.PollResourceInfo.Status.String()
	eventInfo["message"] = se.PollResourceInfo.Message
	return jf.printEvent("status", "resourceStatus", eventInfo)
}

func (jf *formatter) FormatPruneEvent(pe event.PruneEvent) error {
	eventInfo := jf.baseResourceEvent(pe.Identifier)
	if pe.Error != nil {
		eventInfo["error"] = pe.Error.Error()
		// skipped prune sets both error and operation
		if pe.Operation != event.PruneSkipped {
			return jf.printEvent("prune", "resourceFailed", eventInfo)
		}
	}
	eventInfo["operation"] = pe.Operation.String()
	return jf.printEvent("prune", "resourcePruned", eventInfo)
}

func (jf *formatter) FormatDeleteEvent(de event.DeleteEvent) error {
	eventInfo := jf.baseResourceEvent(de.Identifier)
	if de.Error != nil {
		eventInfo["error"] = de.Error.Error()
		// skipped delete sets both error and operation
		if de.Operation != event.DeleteSkipped {
			return jf.printEvent("delete", "resourceFailed", eventInfo)
		}
	}
	eventInfo["operation"] = de.Operation.String()
	return jf.printEvent("delete", "resourceDeleted", eventInfo)
}

func (jf *formatter) FormatWaitEvent(we event.WaitEvent) error {
	eventInfo := jf.baseResourceEvent(we.Identifier)
	eventInfo["operation"] = we.Operation.String()
	return jf.printEvent("wait", "resourceReconciled", eventInfo)
}

func (jf *formatter) FormatErrorEvent(ee event.ErrorEvent) error {
	return jf.printEvent("error", "error", map[string]interface{}{
		"error": ee.Err.Error(),
	})
}

func (jf *formatter) FormatActionGroupEvent(
	age event.ActionGroupEvent,
	ags []event.ActionGroup,
	s stats.Stats,
	_ list.Collector,
) error {
	if age.Action == event.ApplyAction && age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		as := s.ApplyStats
		if err := jf.printEvent("apply", "completed", map[string]interface{}{
			"count":           as.Sum(),
			"createdCount":    as.Created,
			"unchangedCount":  as.Unchanged,
			"configuredCount": as.Configured,
			"serverSideCount": as.ServersideApplied,
			"failedCount":     as.Failed,
		}); err != nil {
			return err
		}
	}

	if age.Action == event.PruneAction && age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		ps := s.PruneStats
		return jf.printEvent("prune", "completed", map[string]interface{}{
			"pruned":  ps.Pruned,
			"skipped": ps.Skipped,
			"failed":  ps.Failed,
		})
	}

	if age.Action == event.DeleteAction && age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		ds := s.DeleteStats
		return jf.printEvent("delete", "completed", map[string]interface{}{
			"deleted": ds.Deleted,
			"skipped": ds.Skipped,
			"failed":  ds.Failed,
		})
	}

	if age.Action == event.WaitAction && age.Type == event.Finished &&
		list.IsLastActionGroup(age, ags) {
		ws := s.WaitStats
		return jf.printEvent("wait", "completed", map[string]interface{}{
			"reconciled": ws.Reconciled,
			"skipped":    ws.Skipped,
			"timeout":    ws.Timeout,
			"failed":     ws.Failed,
		})
	}

	return nil
}

func (jf *formatter) baseResourceEvent(identifier object.ObjMetadata) map[string]interface{} {
	return map[string]interface{}{
		"group":     identifier.GroupKind.Group,
		"kind":      identifier.GroupKind.Kind,
		"namespace": identifier.Namespace,
		"name":      identifier.Name,
	}
}

func (jf *formatter) printEvent(t, eventType string, content map[string]interface{}) error {
	m := make(map[string]interface{})
	m["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	m["type"] = t
	m["eventType"] = eventType
	for key, val := range content {
		m[key] = val
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(jf.ioStreams.Out, string(b)+"\n")
	return err
}

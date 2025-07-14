// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"fmt"
	"maps"
	"time"

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
		now:       time.Now,
	}
}

type formatter struct {
	ioStreams genericiooptions.IOStreams
	now       func() time.Time
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
	objects := make([]any, len(ve.Identifiers))
	for i, id := range ve.Identifiers {
		objects[i] = jf.baseResourceEvent(id)
	}
	return jf.printEvent("validation", map[string]any{
		"objects": objects,
		"error":   err.Error(),
	})
}

func (jf *formatter) FormatApplyEvent(e event.ApplyEvent) error {
	eventInfo := jf.baseResourceEvent(e.Identifier)
	if e.Error != nil {
		eventInfo["error"] = e.Error.Error()
	}
	eventInfo["status"] = e.Status.String()
	return jf.printEvent("apply", eventInfo)
}

func (jf *formatter) FormatStatusEvent(se event.StatusEvent) error {
	return jf.printResourceStatus(se)
}

func (jf *formatter) printResourceStatus(se event.StatusEvent) error {
	eventInfo := jf.baseResourceEvent(se.Identifier)
	eventInfo["status"] = se.PollResourceInfo.Status.String()
	eventInfo["message"] = se.PollResourceInfo.Message
	return jf.printEvent("status", eventInfo)
}

func (jf *formatter) FormatPruneEvent(e event.PruneEvent) error {
	eventInfo := jf.baseResourceEvent(e.Identifier)
	if e.Error != nil {
		eventInfo["error"] = e.Error.Error()
	}
	eventInfo["status"] = e.Status.String()
	return jf.printEvent("prune", eventInfo)
}

func (jf *formatter) FormatDeleteEvent(e event.DeleteEvent) error {
	eventInfo := jf.baseResourceEvent(e.Identifier)
	if e.Error != nil {
		eventInfo["error"] = e.Error.Error()
	}
	eventInfo["status"] = e.Status.String()
	return jf.printEvent("delete", eventInfo)
}

func (jf *formatter) FormatWaitEvent(e event.WaitEvent) error {
	eventInfo := jf.baseResourceEvent(e.Identifier)
	eventInfo["status"] = e.Status.String()
	return jf.printEvent("wait", eventInfo)
}

func (jf *formatter) FormatErrorEvent(e event.ErrorEvent) error {
	return jf.printEvent("error", map[string]any{
		"error": e.Err.Error(),
	})
}

func (jf *formatter) FormatActionGroupEvent(
	age event.ActionGroupEvent,
	ags []event.ActionGroup,
	s stats.Stats,
	_ list.Collector,
) error {
	content := map[string]any{
		"action": age.Action.String(),
		"status": age.Status.String(),
	}

	switch age.Action {
	case event.ApplyAction:
		if age.Status == event.Finished {
			as := s.ApplyStats
			content["count"] = as.Sum()
			content["successful"] = as.Successful
			content["skipped"] = as.Skipped
			content["failed"] = as.Failed
		}
	case event.PruneAction:
		if age.Status == event.Finished {
			ps := s.PruneStats
			content["count"] = ps.Sum()
			content["successful"] = ps.Successful
			content["skipped"] = ps.Skipped
			content["failed"] = ps.Failed
		}
	case event.DeleteAction:
		if age.Status == event.Finished {
			ds := s.DeleteStats
			content["count"] = ds.Sum()
			content["successful"] = ds.Successful
			content["skipped"] = ds.Skipped
			content["failed"] = ds.Failed
		}
	case event.WaitAction:
		if age.Status == event.Finished {
			ws := s.WaitStats
			content["count"] = ws.Sum()
			content["successful"] = ws.Successful
			content["skipped"] = ws.Skipped
			content["failed"] = ws.Failed
			content["timeout"] = ws.Timeout
		}
	case event.InventoryAction:
		// no extra content
	default:
		return fmt.Errorf("invalid action group action: %+v", age)
	}

	return jf.printEvent("group", content)
}

func (jf *formatter) FormatSummary(s stats.Stats) error {
	if s.ApplyStats != (stats.ApplyStats{}) {
		as := s.ApplyStats
		err := jf.printEvent("summary", map[string]any{
			"action":     event.ApplyAction.String(),
			"count":      as.Sum(),
			"successful": as.Successful,
			"skipped":    as.Skipped,
			"failed":     as.Failed,
		})
		if err != nil {
			return err
		}
	}
	if s.PruneStats != (stats.PruneStats{}) {
		ps := s.PruneStats
		err := jf.printEvent("summary", map[string]any{
			"action":     event.PruneAction.String(),
			"count":      ps.Sum(),
			"successful": ps.Successful,
			"skipped":    ps.Skipped,
			"failed":     ps.Failed,
		})
		if err != nil {
			return err
		}
	}
	if s.DeleteStats != (stats.DeleteStats{}) {
		ds := s.DeleteStats
		err := jf.printEvent("summary", map[string]any{
			"action":     event.DeleteAction.String(),
			"count":      ds.Sum(),
			"successful": ds.Successful,
			"skipped":    ds.Skipped,
			"failed":     ds.Failed,
		})
		if err != nil {
			return err
		}
	}
	if s.WaitStats != (stats.WaitStats{}) {
		ws := s.WaitStats
		err := jf.printEvent("summary", map[string]any{
			"action":     event.WaitAction.String(),
			"count":      ws.Sum(),
			"successful": ws.Successful,
			"skipped":    ws.Skipped,
			"failed":     ws.Failed,
			"timeout":    ws.Timeout,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (jf *formatter) baseResourceEvent(identifier object.ObjMetadata) map[string]any {
	return map[string]any{
		"group":     identifier.GroupKind.Group,
		"kind":      identifier.GroupKind.Kind,
		"namespace": identifier.Namespace,
		"name":      identifier.Name,
	}
}

func (jf *formatter) printEvent(t string, content map[string]any) error {
	m := make(map[string]any)
	m["timestamp"] = jf.now().UTC().Format(time.RFC3339)
	m["type"] = t
	maps.Copy(m, content)
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(jf.ioStreams.Out, string(b)+"\n")
	return err
}

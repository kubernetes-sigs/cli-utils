// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/list"
)

func NewFormatter(ioStreams genericclioptions.IOStreams,
	previewStrategy common.DryRunStrategy) list.Formatter {
	return &formatter{
		ioStreams:       ioStreams,
		previewStrategy: previewStrategy,
	}
}

type formatter struct {
	previewStrategy common.DryRunStrategy
	ioStreams       genericclioptions.IOStreams
}

func (jf *formatter) FormatApplyEvent(ae event.ApplyEvent, as *list.ApplyStats, c list.Collector) error {
	switch ae.Type {
	case event.ApplyEventCompleted:
		if err := jf.printEvent("apply", "completed", map[string]interface{}{
			"count":           as.Sum(),
			"createdCount":    as.Created,
			"unchangedCount":  as.Unchanged,
			"configuredCount": as.Configured,
			"serverSideCount": as.ServersideApplied,
		}); err != nil {
			return err
		}

		for id, se := range c.LatestStatus() {
			if err := jf.printResourceStatus(id, se); err != nil {
				return err
			}
		}
	case event.ApplyEventResourceUpdate:
		obj := ae.Object
		gvk := obj.GetObjectKind().GroupVersionKind()
		return jf.printEvent("apply", "resourceApplied", map[string]interface{}{
			"group":     gvk.Group,
			"kind":      gvk.Kind,
			"namespace": getNamespace(obj),
			"name":      getName(obj),
			"operation": ae.Operation.String(),
		})
	}
	return nil
}

func (jf *formatter) FormatStatusEvent(se pollevent.Event, _ list.Collector) error {
	switch se.EventType {
	case pollevent.ResourceUpdateEvent:
		id := se.Resource.Identifier
		return jf.printResourceStatus(id, se)
	case pollevent.ErrorEvent:
		id := se.Resource.Identifier
		return jf.printEvent("status", "error", map[string]interface{}{
			"group":     id.GroupKind.Group,
			"kind":      id.GroupKind.Kind,
			"namespace": id.Namespace,
			"name":      id.Name,
			"error":     se.Error.Error(),
		})
	case pollevent.CompletedEvent:
		return jf.printEvent("status", "completed", map[string]interface{}{})
	}
	return nil
}

func (jf *formatter) printResourceStatus(id object.ObjMetadata, se pollevent.Event) error {
	return jf.printEvent("status", "resourceStatus",
		map[string]interface{}{
			"group":     id.GroupKind.Group,
			"kind":      id.GroupKind.Kind,
			"namespace": id.Namespace,
			"name":      id.Name,
			"status":    se.Resource.Status.String(),
			"message":   se.Resource.Message,
		})
}

func (jf *formatter) FormatPruneEvent(pe event.PruneEvent, ps *list.PruneStats) error {
	switch pe.Type {
	case event.PruneEventCompleted:
		return jf.printEvent("prune", "completed", map[string]interface{}{
			"pruned":  ps.Pruned,
			"skipped": ps.Skipped,
		})
	case event.PruneEventResourceUpdate:
		obj := pe.Object
		gvk := obj.GetObjectKind().GroupVersionKind()
		return jf.printEvent("prune", "resourcePruned", map[string]interface{}{
			"group":     gvk.Group,
			"kind":      gvk.Kind,
			"namespace": getNamespace(obj),
			"name":      getName(obj),
			"operation": pe.Operation.String(),
		})
	}
	return nil
}

func (jf *formatter) FormatDeleteEvent(de event.DeleteEvent, ds *list.DeleteStats) error {
	switch de.Type {
	case event.DeleteEventCompleted:
		return jf.printEvent("delete", "completed", map[string]interface{}{
			"deleted": ds.Deleted,
			"skipped": ds.Skipped,
		})
	case event.DeleteEventResourceUpdate:
		obj := de.Object
		gvk := obj.GetObjectKind().GroupVersionKind()
		return jf.printEvent("delete", "resourceDeleted", map[string]interface{}{
			"group":     gvk.Group,
			"kind":      gvk.Kind,
			"namespace": getNamespace(obj),
			"name":      getName(obj),
			"operation": de.Operation.String(),
		})
	}
	return nil
}

func (jf *formatter) FormatErrorEvent(ee event.ErrorEvent) error {
	return jf.printEvent("error", "error", map[string]interface{}{
		"error": ee.Err.Error(),
	})
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

func getName(obj runtime.Object) string {
	acc, _ := meta.Accessor(obj)
	return acc.GetName()
}

func getNamespace(obj runtime.Object) string {
	acc, _ := meta.Accessor(obj)
	return acc.GetNamespace()
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/list"
)

func NewFormatter(ioStreams genericclioptions.IOStreams,
	previewStrategy common.DryRunStrategy) list.Formatter {
	return &formatter{
		print: getPrintFunc(ioStreams.Out, previewStrategy),
	}
}

type formatter struct {
	print printFunc
}

func (ef *formatter) FormatApplyEvent(ae event.ApplyEvent, as *list.ApplyStats, c list.Collector) error {
	switch ae.Type {
	case event.ApplyEventCompleted:
		output := fmt.Sprintf("%d resource(s) applied. %d created, %d unchanged, %d configured",
			as.Sum(), as.Created, as.Unchanged, as.Configured)
		// Only print information about serverside apply if some of the
		// resources actually were applied serverside.
		if as.ServersideApplied > 0 {
			output += fmt.Sprintf(", %d serverside applied", as.ServersideApplied)
		}
		ef.print(output)
		for id, se := range c.LatestStatus() {
			ef.printResourceStatus(id, se)
		}
	case event.ApplyEventResourceUpdate:
		obj := ae.Object
		gvk := obj.GetObjectKind().GroupVersionKind()
		name := getName(obj)
		ef.print("%s %s", resourceIDToString(gvk.GroupKind(), name),
			strings.ToLower(ae.Operation.String()))
	}
	return nil
}

func (ef *formatter) FormatStatusEvent(se event.StatusEvent, _ list.Collector) error {
	if se.Type == event.StatusEventResourceUpdate {
		id := se.Resource.Identifier
		ef.printResourceStatus(id, se)
	}
	return nil
}

func (ef *formatter) FormatPruneEvent(pe event.PruneEvent, ps *list.PruneStats) error {
	switch pe.Type {
	case event.PruneEventCompleted:
		ef.print("%d resource(s) pruned, %d skipped", ps.Pruned, ps.Skipped)
	case event.PruneEventResourceUpdate:
		obj := pe.Object
		gvk := obj.GetObjectKind().GroupVersionKind()
		name := getName(obj)
		switch pe.Operation {
		case event.Pruned:
			ef.print("%s %s", resourceIDToString(gvk.GroupKind(), name), "pruned")
		case event.PruneSkipped:
			ef.print("%s %s", resourceIDToString(gvk.GroupKind(), name), "prune skipped")
		}
	}
	return nil
}

func (ef *formatter) FormatDeleteEvent(de event.DeleteEvent, ds *list.DeleteStats) error {
	switch de.Type {
	case event.DeleteEventCompleted:
		ef.print("%d resource(s) deleted, %d skipped", ds.Deleted, ds.Skipped)
	case event.DeleteEventResourceUpdate:
		obj := de.Object
		gvk := obj.GetObjectKind().GroupVersionKind()
		name := getName(obj)
		switch de.Operation {
		case event.Deleted:
			ef.print("%s %s", resourceIDToString(gvk.GroupKind(), name), "deleted")
		case event.DeleteSkipped:
			ef.print("%s %s", resourceIDToString(gvk.GroupKind(), name), "delete skipped")
		}
	case event.DeleteEventFailed:
		ef.print("%s %s", resourceIDToString(de.Identifier.GroupKind, de.Identifier.Name), "deletion failed")
	}
	return nil
}

func (ef *formatter) FormatErrorEvent(_ event.ErrorEvent) error {
	return nil
}

func (ef *formatter) printResourceStatus(id object.ObjMetadata, se event.StatusEvent) {
	ef.print("%s is %s: %s", resourceIDToString(id.GroupKind, id.Name),
		se.Resource.Status.String(), se.Resource.Message)
}

func getName(obj runtime.Object) string {
	if acc, err := meta.Accessor(obj); err == nil {
		if n := acc.GetName(); len(n) > 0 {
			return n
		}
	}
	return "<unknown>"
}

// resourceIDToString returns the string representation of a GroupKind and a resource name.
func resourceIDToString(gk schema.GroupKind, name string) string {
	return fmt.Sprintf("%s/%s", strings.ToLower(gk.String()), name)
}

type printFunc func(format string, a ...interface{})

func getPrintFunc(w io.Writer, previewStrategy common.DryRunStrategy) printFunc {
	return func(format string, a ...interface{}) {
		if previewStrategy.ClientDryRun() {
			format += " (preview)"
		} else if previewStrategy.ServerDryRun() {
			format += " (preview-server)"
		}
		fmt.Fprintf(w, format+"\n", a...)
	}
}

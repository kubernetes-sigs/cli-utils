// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
)

// BasicPrinter is a simple implementation that just prints the events
// from the channel in the default format for kubectl.
// We need to support different printers for different output formats.
type BasicPrinter struct {
	IOStreams genericclioptions.IOStreams
}

type applyStats struct {
	serversideApplied int
	created           int
	unchanged         int
	configured        int
}

func (a *applyStats) inc(op event.ApplyEventOperation) {
	switch op {
	case event.ServersideApplied:
		a.serversideApplied++
	case event.Created:
		a.created++
	case event.Unchanged:
		a.unchanged++
	case event.Configured:
		a.configured++
	default:
		panic(fmt.Errorf("unknown apply operation %s", op.String()))
	}
}

func (a *applyStats) sum() int {
	return a.serversideApplied + a.configured + a.unchanged + a.created
}

// Print outputs the events from the provided channel in a simple
// format on StdOut. As we support other printer implementations
// this should probably be an interface.
// This function will block until the channel is closed.
func (b *BasicPrinter) Print(ch <-chan event.Event, preview bool) {
	printFunc := b.getPrintFunc(preview)
	applyStats := &applyStats{}
	pruneCount := 0
	deleteCount := 0
	for e := range ch {
		switch e.Type {
		case event.ErrorType:
			cmdutil.CheckErr(e.ErrorEvent.Err)
		case event.ApplyType:
			ae := e.ApplyEvent
			if ae.Type == event.ApplyEventCompleted {
				output := fmt.Sprintf("%d resource(s) applied. %d created, %d unchanged, %d configured",
					applyStats.sum(), applyStats.created, applyStats.unchanged, applyStats.configured)
				// Only print information about serverside apply if some of the
				// resources actually were applied serverside.
				if applyStats.serversideApplied > 0 {
					output += fmt.Sprintf(", %d serverside applied", applyStats.serversideApplied)
				}
				printFunc(output)
			} else {
				obj := ae.Object
				gvk := obj.GetObjectKind().GroupVersionKind()
				name := getName(obj)
				applyStats.inc(ae.Operation)
				printFunc("%s %s", resourceIDToString(gvk.GroupKind(), name),
					strings.ToLower(ae.Operation.String()))
			}
		case event.StatusType:
			statusEvent := e.StatusEvent
			switch statusEvent.EventType {
			case pollevent.ResourceUpdateEvent:
				id := statusEvent.Resource.Identifier
				gk := id.GroupKind
				printFunc("%s is %s: %s", resourceIDToString(gk, id.Name),
					statusEvent.Resource.Status.String(), statusEvent.Resource.Message)
			case pollevent.ErrorEvent:
				id := statusEvent.Resource.Identifier
				gk := id.GroupKind
				printFunc("%s error: %s\n", resourceIDToString(gk, id.Name),
					statusEvent.Error.Error())
			case pollevent.CompletedEvent:
				printFunc("all resources has reached the Testing status")
			case pollevent.AbortedEvent:
				printFunc("resources failed to the reached Current status")
			}
		case event.PruneType:
			pe := e.PruneEvent
			if pe.Type == event.PruneEventCompleted {
				printFunc("%d resource(s) pruned", pruneCount)
			} else {
				obj := e.PruneEvent.Object
				gvk := obj.GetObjectKind().GroupVersionKind()
				name := getName(obj)
				pruneCount++
				printFunc("%s %s", resourceIDToString(gvk.GroupKind(), name), "pruned")
			}
		case event.DeleteType:
			de := e.DeleteEvent
			if de.Type == event.DeleteEventCompleted {
				printFunc("%d resource(s) deleted", deleteCount)
			} else {
				obj := de.Object
				gvk := obj.GetObjectKind().GroupVersionKind()
				name := getName(obj)
				deleteCount++
				printFunc("%s %s", resourceIDToString(gvk.GroupKind(), name), "deleted")
			}
		}
	}
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

func (b *BasicPrinter) getPrintFunc(preview bool) func(format string, a ...interface{}) {
	return func(format string, a ...interface{}) {
		if preview {
			format += " (preview)"
		}
		fmt.Fprintf(b.IOStreams.Out, format+"\n", a...)
	}
}

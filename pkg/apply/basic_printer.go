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
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

// BasicPrinter is a simple implementation that just prints the events
// from the channel in the default format for kubectl.
// We need to support different printers for different output formats.
type BasicPrinter struct {
	IOStreams genericclioptions.IOStreams
}

// Print outputs the events from the provided channel in a simple
// format on StdOut. As we support other printer implementations
// this should probably be an interface.
// This function will block until the channel is closed.
func (b *BasicPrinter) Print(ch <-chan event.Event) {
	for e := range ch {
		switch e.Type {
		case event.ErrorType:
			cmdutil.CheckErr(e.ErrorEvent.Err)
		case event.ApplyType:
			ae := e.ApplyEvent
			if ae.Type == event.ApplyEventCompleted {
				fmt.Fprintf(b.IOStreams.Out, "all resources have been applied\n")
			} else {
				obj := ae.Object
				gvk := obj.GetObjectKind().GroupVersionKind()
				name := getName(obj)
				fmt.Fprintf(b.IOStreams.Out, "%s %s\n", resourceIDToString(gvk.GroupKind(), name),
					strings.ToLower(ae.Operation.String()))
			}
		case event.StatusType:
			statusEvent := e.StatusEvent
			switch statusEvent.Type {
			case wait.ResourceUpdate:
				id := statusEvent.EventResource.ResourceIdentifier
				gk := id.GroupKind
				fmt.Fprintf(b.IOStreams.Out, "%s is %s: %s\n", resourceIDToString(gk, id.Name),
					statusEvent.EventResource.Status.String(), statusEvent.EventResource.Message)
			case wait.Completed:
				fmt.Fprint(b.IOStreams.Out, "all resources has reached the Current status\n")
			case wait.Aborted:
				fmt.Fprintf(b.IOStreams.Out, "resources failed to the reached Current status\n")
			}
		case event.PruneType:
			pe := e.PruneEvent
			if pe.Type == event.PruneEventCompleted {
				fmt.Fprintf(b.IOStreams.Out, "prune completed\n")
			} else {
				obj := e.PruneEvent.Object
				gvk := obj.GetObjectKind().GroupVersionKind()
				name := getName(obj)
				fmt.Fprintf(b.IOStreams.Out, "%s %s\n", resourceIDToString(gvk.GroupKind(), name), "pruned")
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

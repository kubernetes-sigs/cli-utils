// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

// Color codes
const (
	StartGreen = "\033[1;32m"
	StartRed = "\033[1;31m"
	StartYellow = "\033[1;33m"
	ResetColor = "\033[0m"
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
func (b *BasicPrinter) Print(ch <-chan Event, applier *Applier) {
	if (applier.Preview) {
		b.PrintPreviewEvents(ch)
		return
	}
	for event := range ch {
		switch event.EventType {
		case ErrorEventType:
			cmdutil.CheckErr(event.ErrorEvent.Err)
		case ApplyEventType:
			obj := event.ApplyEvent.Object
			gvk := obj.GetObjectKind().GroupVersionKind()
			name := "<unknown>"
			if acc, err := meta.Accessor(obj); err == nil {
				if n := acc.GetName(); len(n) > 0 {
					name = n
				}
			}
			fmt.Fprintf(b.IOStreams.Out, "%s %s\n", resourceIDToString(gvk.GroupKind(), name), event.ApplyEvent.Operation)
		case StatusEventType:
			statusEvent := event.StatusEvent
			switch statusEvent.Type {
			case wait.ResourceUpdate:
				id := statusEvent.EventResource.ResourceIdentifier
				gk := id.GroupKind
				fmt.Fprintf(b.IOStreams.Out, "%s is %s: %s\n", resourceIDToString(gk, id.Name), statusEvent.EventResource.Status.String(), statusEvent.EventResource.Message)
			case wait.Completed:
				fmt.Fprint(b.IOStreams.Out, "all resources has reached the Current status\n")
			case wait.Aborted:
				fmt.Fprintf(b.IOStreams.Out, "resources failed to the reached Current status\n")
			}
		}
	}
}

// PrintPreviewEvents outputs only preview events from the provided channel in a preview
// format on StdOut.
func (b *BasicPrinter) PrintPreviewEvents(ch <-chan Event) {
	createdCnt := 0
	modifiedCnt := 0
	deletedCnt := 0
	fmt.Fprintf(b.IOStreams.Out, "\nA preview of operations is shown below. Please use apply to perform the operations.\n\n")
	for event := range ch {
		obj := event.ApplyEvent.Object
		gvk := obj.GetObjectKind().GroupVersionKind()
		name := "<unknown>"
		if acc, err := meta.Accessor(obj); err == nil {
			if n := acc.GetName(); len(n) > 0 {
				name = n
			}
		}
		switch event.ApplyEvent.Operation {
		case "created":
			fmt.Fprintf(b.IOStreams.Out, "%s+%s%s %s\n\n", StartGreen, resourceIdInPreviewFmt(gvk, name), ResetColor, event.ApplyEvent.Operation)
			createdCnt++
		case "deleted":
			fmt.Fprintf(b.IOStreams.Out, "%s-%s%s %s\n\n", StartRed, resourceIdInPreviewFmt(gvk, name), ResetColor, event.ApplyEvent.Operation)
			deletedCnt++
		case "unchanged":
			fmt.Fprintf(b.IOStreams.Out, "%s %s\n\n", resourceIdInPreviewFmt(gvk, name), event.ApplyEvent.Operation)
		default:
			fmt.Fprintf(b.IOStreams.Out, "%s~%s%s %s\n\n", StartYellow, resourceIdInPreviewFmt(gvk, name), ResetColor, event.ApplyEvent.Operation)
			modifiedCnt++
		}
	}
	fmt.Fprintf(b.IOStreams.Out, "\nResources: %s%d to create%s, %s%d to modify%s, %s%d to delete%s\n",
		StartGreen, createdCnt, ResetColor, StartYellow, modifiedCnt, ResetColor, StartRed, deletedCnt, ResetColor)
}

// resourceIDToString returns the string representation of a GroupKind and a resource name.
func resourceIDToString(gk schema.GroupKind, name string) string {
	return fmt.Sprintf("%s/%s", strings.ToLower(gk.String()), name)
}

// resourceIdInPreviewFmt returns the string representation of a GroupVersionKind in preview format.
func resourceIdInPreviewFmt(gvk schema.GroupVersionKind, name string) string {
	return fmt.Sprintf("%s.%s.%s", strings.ToLower(gvk.Version), strings.ToLower(gvk.Kind), name)
}

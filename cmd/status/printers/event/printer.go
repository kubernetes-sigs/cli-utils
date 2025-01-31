// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"fmt"
	"strings"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/cli-utils/cmd/status/printers/printer"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/collector"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/list"
	"sigs.k8s.io/cli-utils/pkg/printers/events"
)

// Printer implements the Printer interface and outputs the resource
// status information as a list of events as they happen.
type Printer struct {
	Formatter list.Formatter
	IOStreams genericiooptions.IOStreams
	Data      *printer.PrintData
}

// NewPrinter returns a new instance of the eventPrinter.
func NewPrinter(ioStreams genericiooptions.IOStreams, printData *printer.PrintData) *Printer {
	return &Printer{
		Formatter: events.NewFormatter(ioStreams, common.DryRunNone),
		IOStreams: ioStreams,
		Data:      printData,
	}
}

// Print takes an event channel and outputs the status events on the channel
// until the channel is closed. The provided cancelFunc is consulted on
// every event and is responsible for stopping the poller when appropriate.
// This function will block.
func (ep *Printer) Print(ch <-chan pollevent.Event, identifiers object.ObjMetadataSet,
	cancelFunc collector.ObserverFunc) error {
	coll := collector.NewResourceStatusCollector(identifiers)
	// The actual work is done by the collector, which will invoke the
	// callback on every event. In the callback we print the status
	// information and call the cancelFunc which is responsible for
	// stopping the poller at the correct time.
	done := coll.ListenWithObserver(ch, collector.ObserverFunc(
		func(statusCollector *collector.ResourceStatusCollector, e pollevent.Event) {
			err := ep.printStatusEvent(e)
			if err != nil {
				panic(err)
			}
			cancelFunc(statusCollector, e)
		}),
	)
	// Listen to the channel until it is closed.
	var err error
	for msg := range done {
		err = msg.Err
	}
	return err
}

func (ep *Printer) printStatusEvent(se pollevent.Event) error {
	switch se.Type {
	case pollevent.ResourceUpdateEvent:
		id := se.Resource.Identifier
		var invName fmt.Stringer
		var ok bool
		if invName, ok = ep.Data.InvIDMap[id]; !ok {
			return fmt.Errorf("%s: resource not found", id)
		}
		// filter out status that are not assigned
		statusString := se.Resource.Status.String()
		if _, ok := ep.Data.StatusSet[strings.ToLower(statusString)]; len(ep.Data.StatusSet) != 0 && !ok {
			return nil
		}
		_, err := fmt.Fprintf(ep.IOStreams.Out, "%s/%s/%s/%s is %s: %s\n", invName,
			strings.ToLower(id.GroupKind.String()), id.Namespace, id.Name, statusString, se.Resource.Message)
		return err
	case pollevent.ErrorEvent:
		return ep.Formatter.FormatErrorEvent(event.ErrorEvent{
			Err: se.Error,
		})
	}
	return nil
}

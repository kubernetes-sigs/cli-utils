// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package table

import (
	"fmt"
	"io"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/cmd/status/printers/printer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/collector"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/table"
)

const (
	// updateInterval defines how often the printer will update the UI.
	updateInterval = 1 * time.Second
)

// Printer is an implementation of the Printer interface that outputs
// status information about resources in a table format with in-place updates.
type Printer struct {
	IOStreams genericclioptions.IOStreams
	PrintData *printer.PrintData
}

// NewPrinter returns a new instance of the tablePrinter.
func NewPrinter(ioStreams genericclioptions.IOStreams, printData *printer.PrintData) *Printer {
	return &Printer{
		IOStreams: ioStreams,
		PrintData: printData,
	}
}

// Print take an event channel and outputs the status events on the channel
// until the channel is closed .
func (t *Printer) Print(ch <-chan event.Event, identifiers object.ObjMetadataSet,
	cancelFunc collector.ObserverFunc) error {
	coll := collector.NewResourceStatusCollector(identifiers)
	stop := make(chan struct{})

	// Start the goroutine that is responsible for
	// printing the latest state on a regular cadence.
	printCompleted := t.runPrintLoop(&CollectorAdapter{
		collector:  coll,
		invNameMap: t.PrintData.InvNameMap,
		statusSet:  t.PrintData.StatusSet,
	}, stop)

	// Make the collector start listening on the eventChannel.
	done := coll.ListenWithObserver(ch, cancelFunc)

	// Block until all the collector has shut down. This means the
	// eventChannel has been closed and all events have been processed.
	var err error
	for msg := range done {
		err = msg.Err
	}

	// Close the stop channel to notify the print goroutine that it should
	// shut down.
	close(stop)

	// Wait until the printCompleted channel is closed. This means
	// the printer has updated the UI with the latest state and
	// exited from the goroutine.
	<-printCompleted
	return err
}

var invNameColumn = table.ColumnDef{
	ColumnName:   "inventory_name",
	ColumnHeader: "INVENTORY_NAME",
	ColumnWidth:  30,
	PrintResourceFunc: func(w io.Writer, width int, r table.Resource) (int, error) {
		group := r.(*ResourceInfo).invName
		if len(group) > width {
			group = group[:width]
		}
		_, err := fmt.Fprint(w, group)
		return len(group), err
	},
}

var columns = []table.ColumnDefinition{
	table.MustColumn("namespace"),
	table.MustColumn("resource"),
	table.MustColumn("status"),
	table.MustColumn("conditions"),
	table.MustColumn("age"),
	table.MustColumn("message"),
	invNameColumn,
}

// Print prints the table of resources with their statuses until the
// provided stop channel is closed.
func (t *Printer) runPrintLoop(coll *CollectorAdapter, stop <-chan struct{}) <-chan struct{} {
	finished := make(chan struct{})

	baseTablePrinter := table.BaseTablePrinter{
		IOStreams: t.IOStreams,
		Columns:   columns,
	}

	linesPrinted := baseTablePrinter.PrintTable(coll.LatestStatus(), 0)

	go func() {
		defer close(finished)
		ticker := time.NewTicker(updateInterval)
		for {
			select {
			case <-stop:
				ticker.Stop()
				linesPrinted = baseTablePrinter.PrintTable(
					coll.LatestStatus(), linesPrinted)
				return
			case <-ticker.C:
				linesPrinted = baseTablePrinter.PrintTable(
					coll.LatestStatus(), linesPrinted)
			}
		}
	}()

	return finished
}

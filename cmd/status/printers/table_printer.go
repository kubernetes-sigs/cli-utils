// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printers

import (
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/collector"
	"sigs.k8s.io/cli-utils/pkg/print/table"
)

const (
	// updateInterval defines how often the printer will update the UI.
	updateInterval = 1 * time.Second
)

// tablePrinter is an implementation of the Printer interface that outputs
// status information about resources in a table format with in-place updates.
type tablePrinter struct {
	collector *collector.ResourceStatusCollector
	ioStreams genericclioptions.IOStreams
}

// NewTablePrinter returns a new instance of the tablePrinter. The passed in
// collector is the source of data to be printed, and the writer is where the
// printer will send the output.
func NewTablePrinter(collector *collector.ResourceStatusCollector,
	ioStreams genericclioptions.IOStreams) *tablePrinter {
	return &tablePrinter{
		collector: collector,
		ioStreams: ioStreams,
	}
}

var columns = []table.ColumnDefinition{
	table.MustColumn("namespace"),
	table.MustColumn("resource"),
	table.MustColumn("status"),
	table.MustColumn("conditions"),
	table.MustColumn("age"),
	table.MustColumn("message"),
}

// Print prints the table of resources with their statuses until the
// provided stop channel is closed.
func (t *tablePrinter) Print(stop <-chan struct{}) <-chan struct{} {
	completed := make(chan struct{})

	baseTablePrinter := table.BaseTablePrinter{
		IOStreams: t.ioStreams,
		Columns:   columns,
	}

	collectorAdapter := &CollectorAdapter{
		collector: t.collector,
	}

	linesPrinted := baseTablePrinter.PrintTable(collectorAdapter.LatestStatus(), 0)

	go func() {
		defer close(completed)
		ticker := time.NewTicker(updateInterval)
		for {
			select {
			case <-stop:
				ticker.Stop()
				linesPrinted = baseTablePrinter.PrintTable(
					collectorAdapter.LatestStatus(), linesPrinted)
				return
			case <-ticker.C:
				linesPrinted = baseTablePrinter.PrintTable(
					collectorAdapter.LatestStatus(), linesPrinted)
			}
		}
	}()

	return completed
}

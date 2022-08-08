// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printers

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/cmd/status/printers/event"
	"sigs.k8s.io/cli-utils/cmd/status/printers/printer"
	"sigs.k8s.io/cli-utils/cmd/status/printers/table"
)

// CreatePrinter return an implementation of the Printer interface. The
// actual implementation is based on the printerType requested.
func CreatePrinter(printerType string, ioStreams genericclioptions.IOStreams, printData *printer.PrintData) (printer.Printer, error) {
	switch printerType {
	case "table":
		return table.NewPrinter(ioStreams), nil
	default:
		return event.NewPrinter(ioStreams, printData), nil
	}
}

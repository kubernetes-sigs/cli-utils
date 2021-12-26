// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printers

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/printers/events"
	"sigs.k8s.io/cli-utils/pkg/printers/json"
	"sigs.k8s.io/cli-utils/pkg/printers/printer"
	"sigs.k8s.io/cli-utils/pkg/printers/table"
)

const (
	EventsPrinter = "events"
	TablePrinter  = "table"
	JSONPrinter   = "json"
)

func GetPrinter(printerType string, ioStreams genericclioptions.IOStreams, drs printer.DryRunStringer) printer.Printer {
	switch printerType { //nolint:gocritic
	case TablePrinter:
		return table.NewPrinter(ioStreams)
	case JSONPrinter:
		return json.NewPrinter(ioStreams)
	default:
		return events.NewPrinter(ioStreams, drs)
	}
}

func SupportedPrinters() []string {
	return []string{EventsPrinter, TablePrinter, JSONPrinter}
}

func DefaultPrinter() string {
	return EventsPrinter
}

func ValidatePrinterType(printerType string) bool {
	for _, p := range SupportedPrinters() {
		if printerType == p {
			return true
		}
	}
	return false
}

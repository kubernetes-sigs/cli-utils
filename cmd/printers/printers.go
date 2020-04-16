// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package printers

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/cmd/printers/printer"
	"sigs.k8s.io/cli-utils/cmd/printers/table"
	"sigs.k8s.io/cli-utils/pkg/apply"
)

const (
	EventsPrinter = "events"
	TablePrinter  = "table"
)

func GetPrinter(printerType string, ioStreams genericclioptions.IOStreams) printer.Printer {
	switch printerType { //nolint:gocritic
	case TablePrinter:
		return &table.Printer{
			IOStreams: ioStreams,
		}
	default:
		return &apply.BasicPrinter{
			IOStreams: ioStreams,
		}
	}
}

func SupportedPrinters() []string {
	return []string{EventsPrinter, TablePrinter}
}

func DefaultPrinter() string {
	return EventsPrinter
}

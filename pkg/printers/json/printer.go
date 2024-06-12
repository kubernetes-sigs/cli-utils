// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/print/list"
	"sigs.k8s.io/cli-utils/pkg/printers/printer"
)

func NewPrinter(ioStreams genericiooptions.IOStreams) printer.Printer {
	return &list.BaseListPrinter{
		FormatterFactory: func(previewStrategy common.DryRunStrategy) list.Formatter {
			return NewFormatter(ioStreams, previewStrategy)
		},
	}
}

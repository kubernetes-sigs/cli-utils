// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"testing"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/cli-utils/pkg/printers/printer"
	printertesting "sigs.k8s.io/cli-utils/pkg/printers/testutil"
)

func TestPrint(t *testing.T) {
	printertesting.PrintResultErrorTest(t, func() printer.Printer {
		ioStreams, _, _, _ := genericiooptions.NewTestIOStreams()
		return NewPrinter(ioStreams)
	})
}

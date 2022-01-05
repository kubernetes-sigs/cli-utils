// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package list

import (
	"testing"

	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/print/stats"
	"sigs.k8s.io/cli-utils/pkg/printers/printer"
	printertesting "sigs.k8s.io/cli-utils/pkg/printers/testutil"
)

func TestPrint(t *testing.T) {
	printertesting.PrintResultErrorTest(t, func() printer.Printer {
		return &BaseListPrinter{
			FormatterFactory: func(previewStrategy common.DryRunStrategy) Formatter {
				return newCountingFormatter()
			},
		}
	})
}

func newCountingFormatter() *countingFormatter {
	return &countingFormatter{}
}

type countingFormatter struct {
	applyEvents      []event.ApplyEvent
	statusEvents     []event.StatusEvent
	pruneEvents      []event.PruneEvent
	deleteEvents     []event.DeleteEvent
	waitEvents       []event.WaitEvent
	errorEvent       event.ErrorEvent
	actionGroupEvent []event.ActionGroupEvent
}

func (c *countingFormatter) FormatApplyEvent(e event.ApplyEvent) error {
	c.applyEvents = append(c.applyEvents, e)
	return nil
}

func (c *countingFormatter) FormatStatusEvent(e event.StatusEvent) error {
	c.statusEvents = append(c.statusEvents, e)
	return nil
}

func (c *countingFormatter) FormatPruneEvent(e event.PruneEvent) error {
	c.pruneEvents = append(c.pruneEvents, e)
	return nil
}

func (c *countingFormatter) FormatDeleteEvent(e event.DeleteEvent) error {
	c.deleteEvents = append(c.deleteEvents, e)
	return nil
}

func (c *countingFormatter) FormatWaitEvent(e event.WaitEvent) error {
	c.waitEvents = append(c.waitEvents, e)
	return nil
}

func (c *countingFormatter) FormatErrorEvent(e event.ErrorEvent) error {
	c.errorEvent = e
	return nil
}

func (c *countingFormatter) FormatActionGroupEvent(e event.ActionGroupEvent, _ []event.ActionGroup, _ stats.Stats,
	_ Collector) error {
	c.actionGroupEvent = append(c.actionGroupEvent, e)
	return nil
}

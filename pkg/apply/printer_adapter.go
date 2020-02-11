// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"io"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
)

// KubectlPrinterAdapter is a workaround for capturing progress from
// ApplyOptions. ApplyOptions were originally meant to print progress
// directly using a configurable printer. The KubectlPrinterAdapter
// plugs into ApplyOptions as a ToPrinter function, but instead of
// printing the info, it emits it as an event on the provided channel.
type KubectlPrinterAdapter struct {
	ch chan<- event.Event
}

// resourcePrinterImpl implements the ResourcePrinter interface. But
// instead of printing, it emits information on the provided channel.
type resourcePrinterImpl struct {
	operation string
	ch        chan<- event.Event
}

// PrintObj takes the provided object and operation and emits
// it on the channel.
func (r *resourcePrinterImpl) PrintObj(obj runtime.Object, _ io.Writer) error {
	r.ch <- event.Event{
		Type: event.ApplyEventType,
		ApplyEvent: event.ApplyEvent{
			Operation: r.operation,
			Object:    obj,
		},
	}
	return nil
}

type toPrinterFunc func(string) (printers.ResourcePrinter, error)

// toPrinterFunc returns a function of type toPrinterFunc. This
// is the type required by the ApplyOptions.
func (p *KubectlPrinterAdapter) toPrinterFunc() toPrinterFunc {
	return func(operation string) (printers.ResourcePrinter, error) {
		return &resourcePrinterImpl{
			ch:        p.ch,
			operation: operation,
		}, nil
	}
}

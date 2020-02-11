// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
)

// NewDestroyer returns a new destroyer. It will set up the ApplyOptions and
// PruneOptions which are responsible for capturing any command line flags.
// It currently requires IOStreams, but this is a legacy from when
// the ApplyOptions were responsible for printing progress. This is now
// handled by a separate printer with the KubectlPrinterAdapter bridging
// between the two.
func NewDestroyer(factory util.Factory, ioStreams genericclioptions.IOStreams) *Destroyer {
	return &Destroyer{
		ApplyOptions: apply.NewApplyOptions(ioStreams),
		PruneOptions: prune.NewPruneOptions(),
		factory:      factory,
		ioStreams:    ioStreams,
	}
}

// Destroyer performs the step of grabbing all the previous inventory objects and
// prune them. This also deletes all the previous inventory objects
type Destroyer struct {
	factory      util.Factory
	ioStreams    genericclioptions.IOStreams
	ApplyOptions *apply.ApplyOptions
	PruneOptions *prune.PruneOptions

	DryRun bool
}

// Initialize sets up the Destroyer for actually doing an destroy against
// a cluster. This involves validating command line inputs and configuring
// clients for communicating with the cluster.
func (d *Destroyer) Initialize(cmd *cobra.Command) error {
	err := d.ApplyOptions.Complete(d.factory, cmd)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up ApplyOptions", 1)
	}
	err = d.PruneOptions.Initialize(d.factory, d.ApplyOptions.Namespace)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up PruneOptions", 1)
	}

	// Propagate dry-run flags.
	d.ApplyOptions.DryRun = d.DryRun
	d.PruneOptions.DryRun = d.DryRun

	if err != nil {
		return errors.WrapPrefix(err, "error creating resolver", 1)
	}
	return nil
}

// Run performs the destroy step. This happens asynchronously
// on progress and any errors are reported back on the event channel.
func (d *Destroyer) Run() <-chan event.Event {
	ch := make(chan event.Event)

	go func() {
		defer close(ch)
		infos, _ := d.ApplyOptions.GetObjects()
		err := d.PruneOptions.Prune(infos, ch)
		if err != nil {
			// If we see an error here we just report it on the channel and then
			// give up. Eventually we might be able to determine which errors
			// are fatal and which might allow us to continue.
			ch <- event.Event{
				Type: event.ErrorEventType,
				ErrorEvent: event.ErrorEvent{
					Err: errors.WrapPrefix(err, "error pruning resources", 1),
				},
			}
			return
		}
	}()
	return ch
}

// SetFlags configures the command line flags needed for destroy
// This is a temporary solution as we should separate the configuration
// of cobra flags from the Destroyer.
func (d *Destroyer) SetFlags(cmd *cobra.Command) {
	d.ApplyOptions.DeleteFlags.AddFlags(cmd)
	d.ApplyOptions.RecordFlags.AddFlags(cmd)
	d.ApplyOptions.Overwrite = true
}

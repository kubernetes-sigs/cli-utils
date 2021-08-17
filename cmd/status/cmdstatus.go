// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/cmd/flagutils"
	"sigs.k8s.io/cli-utils/cmd/status/printers"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/aggregator"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/collector"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/util/factory"
)

func GetStatusRunner(factory cmdutil.Factory, invFactory inventory.InventoryClientFactory, loader manifestreader.ManifestLoader) *StatusRunner {
	r := &StatusRunner{
		factory:           factory,
		invFactory:        invFactory,
		loader:            loader,
		pollerFactoryFunc: pollerFactoryFunc,
	}
	c := &cobra.Command{
		Use:  "status (DIRECTORY | STDIN)",
		RunE: r.runE,
	}
	c.Flags().DurationVar(&r.period, "poll-period", 2*time.Second,
		"Polling period for resource statuses.")
	c.Flags().StringVar(&r.pollUntil, "poll-until", "known",
		"When to stop polling. Must be one of 'known', 'current', 'deleted', or 'forever'.")
	c.Flags().StringVar(&r.output, "output", "events", "Output format.")
	c.Flags().DurationVar(&r.timeout, "timeout", 0,
		"How long to wait before exiting")

	r.Command = c
	return r
}

func StatusCommand(f cmdutil.Factory, invFactory inventory.InventoryClientFactory, loader manifestreader.ManifestLoader) *cobra.Command {
	return GetStatusRunner(f, invFactory, loader).Command
}

// StatusRunner captures the parameters for the command and contains
// the run function.
type StatusRunner struct {
	Command    *cobra.Command
	factory    cmdutil.Factory
	invFactory inventory.InventoryClientFactory
	loader     manifestreader.ManifestLoader

	period    time.Duration
	pollUntil string
	timeout   time.Duration
	output    string

	pollerFactoryFunc func(cmdutil.Factory) (poller.Poller, error)
}

// runE implements the logic of the command and will delegate to the
// poller to compute status for each of the resources. One of the printer
// implementations takes care of printing the output.
func (r *StatusRunner) runE(cmd *cobra.Command, args []string) error {
	_, err := common.DemandOneDirectory(args)
	if err != nil {
		return err
	}

	reader, err := r.loader.ManifestReader(cmd.InOrStdin(), flagutils.PathFromArgs(args))
	if err != nil {
		return err
	}
	objs, err := reader.Read()
	if err != nil {
		return err
	}

	inv, _, err := inventory.SplitInventory(objs)
	if err != nil {
		return err
	}

	invClient, err := r.invFactory.NewInventoryClient(r.factory)
	if err != nil {
		return err
	}

	// Based on the inventory template manifest we look up the inventory
	// from the live state using the inventory client.
	identifiers, err := invClient.GetClusterObjs(inv, common.DryRunNone)
	if err != nil {
		return err
	}

	// Exit here if the inventory is empty.
	if len(identifiers) == 0 {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "no resources found in the inventory\n")
		return nil
	}

	statusPoller, err := r.pollerFactoryFunc(r.factory)
	if err != nil {
		return err
	}

	// Fetch a printer implementation based on the desired output format as
	// specified in the output flag.
	printer, err := printers.CreatePrinter(r.output, genericclioptions.IOStreams{
		In:     cmd.InOrStdin(),
		Out:    cmd.OutOrStdout(),
		ErrOut: cmd.ErrOrStderr(),
	})
	if err != nil {
		return fmt.Errorf("error creating printer: %w", err)
	}

	// If the user has specified a timeout, we create a context with timeout,
	// otherwise we create a context with cancel.
	ctx := context.Background()
	var cancel func()
	if r.timeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Choose the appropriate ObserverFunc based on the criteria for when
	// the command should exit.
	var cancelFunc collector.ObserverFunc
	switch r.pollUntil {
	case "known":
		cancelFunc = allKnownNotifierFunc(cancel)
	case "current":
		cancelFunc = desiredStatusNotifierFunc(cancel, status.CurrentStatus)
	case "deleted":
		cancelFunc = desiredStatusNotifierFunc(cancel, status.NotFoundStatus)
	case "forever":
		cancelFunc = func(*collector.ResourceStatusCollector, event.Event) {}
	default:
		return fmt.Errorf("unknown value for pollUntil: %q", r.pollUntil)
	}

	eventChannel := statusPoller.Poll(ctx, identifiers, polling.Options{
		PollInterval: r.period,
		UseCache:     true,
	})

	return printer.Print(eventChannel, identifiers, cancelFunc)
}

// desiredStatusNotifierFunc returns an Observer function for the
// ResourceStatusCollector that will cancel the context (using the cancelFunc)
// when all resources have reached the desired status.
func desiredStatusNotifierFunc(cancelFunc context.CancelFunc,
	desired status.Status) collector.ObserverFunc {
	return func(rsc *collector.ResourceStatusCollector, _ event.Event) {
		var rss []*event.ResourceStatus
		for _, rs := range rsc.ResourceStatuses {
			rss = append(rss, rs)
		}
		aggStatus := aggregator.AggregateStatus(rss, desired)
		if aggStatus == desired {
			cancelFunc()
		}
	}
}

// allKnownNotifierFunc returns an Observer function for the
// ResourceStatusCollector that will cancel the context (using the cancelFunc)
// when all resources have a known status.
func allKnownNotifierFunc(cancelFunc context.CancelFunc) collector.ObserverFunc {
	return func(rsc *collector.ResourceStatusCollector, _ event.Event) {
		for _, rs := range rsc.ResourceStatuses {
			if rs.Status == status.UnknownStatus {
				return
			}
		}
		cancelFunc()
	}
}

func pollerFactoryFunc(f cmdutil.Factory) (poller.Poller, error) {
	return factory.NewStatusPoller(f)
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"time"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/cmd/status/printers"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/aggregator"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/collector"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/util/factory"
)

func GetStatusRunner(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *StatusRunner {
	r := &StatusRunner{
		factory:   f,
		ioStreams: ioStreams,
	}
	c := &cobra.Command{
		Use:  "status DIR",
		RunE: r.runE,
	}
	c.Flags().DurationVar(&r.period, "poll-period", 2*time.Second,
		"Polling period for resource statuses.")
	c.Flags().BoolVar(&r.pollForever, "poll-forever", false,
		"Keep polling forever.")
	c.Flags().StringVar(&r.output, "output", "table", "Output format.")
	c.Flags().BoolVar(&r.waitForDeletion, "wait-for-deletion", false,
		"Wait for all resources to be deleted")
	c.Flags().DurationVar(&r.timeout, "timeout", time.Minute,
		"Timeout threshold for waiting for all resources to reach the desired status")

	r.command = c
	return r
}

func StatusCommand(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetStatusRunner(f, ioStreams).command
}

// WaitRunner captures the parameters for the command and contains
// the run function.
type StatusRunner struct {
	command   *cobra.Command
	ioStreams genericclioptions.IOStreams
	factory   cmdutil.Factory

	period          time.Duration
	timeout         time.Duration
	pollForever     bool
	output          string
	waitForDeletion bool
}

// runE implements the logic of the command and will call the Wait command in
// the wait package, use a ResourceStatusCollector to capture the events from
// the channel, and the tablePrinter to display the information.
func (r *StatusRunner) runE(cmd *cobra.Command, args []string) error {
	_, err := common.DemandOneDirectory(args)
	if err != nil {
		return err
	}

	var reader manifestreader.ManifestReader
	readerOptions := manifestreader.ReaderOptions{
		Factory:   r.factory,
		Namespace: metav1.NamespaceDefault,
	}
	if len(args) == 0 {
		reader = &manifestreader.StreamManifestReader{
			ReaderName:    "stdin",
			Reader:        cmd.InOrStdin(),
			ReaderOptions: readerOptions,
		}
	} else {
		reader = &manifestreader.PathManifestReader{
			Path:          args[0],
			ReaderOptions: readerOptions,
		}
	}
	infos, err := reader.Read()
	if err != nil {
		return err
	}

	identifiers := object.InfosToObjMetas(infos)

	statusPoller, err := factory.NewStatusPoller(r.factory)
	if err != nil {
		return err
	}

	coll := collector.NewResourceStatusCollector(identifiers)
	stop := make(chan struct{})
	printer, err := printers.CreatePrinter(r.output, coll, r.ioStreams)
	if err != nil {
		return errors.WrapPrefix(err, "error creating printer", 1)
	}
	printingFinished := printer.Print(stop)

	ctx := context.Background()

	var completed <-chan struct{}
	if r.pollForever {
		eventChannel := statusPoller.Poll(ctx, identifiers, polling.Options{
			PollInterval: r.period,
			UseCache:     true,
		})
		completed = coll.Listen(eventChannel)
	} else {
		ctx, cancel := context.WithTimeout(ctx, r.timeout)

		var desiredStatus status.Status
		if r.waitForDeletion {
			desiredStatus = status.NotFoundStatus
		} else {
			desiredStatus = status.CurrentStatus
		}
		eventChannel := statusPoller.Poll(ctx, identifiers, polling.Options{
			PollInterval: r.period,
			UseCache:     true,
		})
		completed = coll.ListenWithObserver(eventChannel, getNotifierFunc(cancel, desiredStatus))
	}

	// Wait for the collector to finish. This will happen when the event
	// channel is closed.
	<-completed
	// Close the stop channel to notify the printer that it should shut down.
	close(stop)
	// Wait for the printer to print the latest state before exiting the program.
	<-printingFinished
	return nil
}

// getNotifierFunc returns a notifier function for the ResourceStatusCollector
// that will cancel the context (using the cancelFunc) when all resources
// have reached the desired status.
func getNotifierFunc(cancelFunc context.CancelFunc, desired status.Status) collector.ObserverFunc {
	return func(rsc *collector.ResourceStatusCollector) {
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

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package destroy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/cmd/flagutils"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/printers"
)

// GetRunner creates and returns the Runner which stores the cobra command.
func GetRunner(factory cmdutil.Factory, invFactory inventory.ClientFactory,
	loader manifestreader.ManifestLoader, ioStreams genericclioptions.IOStreams) *Runner {
	r := &Runner{
		ioStreams:  ioStreams,
		factory:    factory,
		invFactory: invFactory,
		loader:     loader,
	}
	cmd := &cobra.Command{
		Use:                   "destroy (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Destroy all the resources related to configuration"),
		RunE:                  r.RunE,
	}

	cmd.Flags().StringVar(&r.output, "output", printers.DefaultPrinter(),
		fmt.Sprintf("Output format, must be one of %s", strings.Join(printers.SupportedPrinters(), ",")))
	cmd.Flags().StringVar(&r.inventoryPolicy, flagutils.InventoryPolicyFlag, flagutils.InventoryPolicyStrict,
		"It determines the behavior when the resources don't belong to current inventory. Available options "+
			fmt.Sprintf("%q, %q and %q.", flagutils.InventoryPolicyStrict, flagutils.InventoryPolicyAdopt, flagutils.InventoryPolicyForceAdopt))
	cmd.Flags().DurationVar(&r.deleteTimeout, "delete-timeout", time.Duration(0),
		"Timeout threshold for waiting for all deleted resources to complete deletion")
	cmd.Flags().StringVar(&r.deletePropagationPolicy, "delete-propagation-policy",
		"Background", "Propagation policy for deletion")
	cmd.Flags().DurationVar(&r.timeout, "timeout", 0,
		"How long to wait before exiting")
	cmd.Flags().BoolVar(&r.printStatusEvents, "status-events", false,
		"Print status events (always enabled for table output)")

	r.Command = cmd
	return r
}

// Command creates the Runner, returning the cobra command associated with it.
func Command(f cmdutil.Factory, invFactory inventory.ClientFactory, loader manifestreader.ManifestLoader,
	ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetRunner(f, invFactory, loader, ioStreams).Command
}

// Runner encapsulates data necessary to run the destroy command.
type Runner struct {
	Command    *cobra.Command
	ioStreams  genericclioptions.IOStreams
	factory    cmdutil.Factory
	invFactory inventory.ClientFactory
	loader     manifestreader.ManifestLoader

	output                  string
	deleteTimeout           time.Duration
	deletePropagationPolicy string
	inventoryPolicy         string
	timeout                 time.Duration
	printStatusEvents       bool
}

func (r *Runner) RunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// If specified, cancel with timeout.
	if r.timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	deletePropPolicy, err := flagutils.ConvertPropagationPolicy(r.deletePropagationPolicy)
	if err != nil {
		return err
	}
	inventoryPolicy, err := flagutils.ConvertInventoryPolicy(r.inventoryPolicy)
	if err != nil {
		return err
	}

	if found := printers.ValidatePrinterType(r.output); !found {
		return fmt.Errorf("unknown output type %q", r.output)
	}

	// Retrieve the inventory object.
	reader, err := r.loader.ManifestReader(cmd.InOrStdin(), flagutils.PathFromArgs(args))
	if err != nil {
		return err
	}
	objs, err := reader.Read()
	if err != nil {
		return err
	}
	invObj, _, err := inventory.SplitUnstructureds(objs)
	if err != nil {
		return err
	}
	inv := inventory.InfoFromObject(invObj)

	invClient, err := r.invFactory.NewClient(r.factory)
	if err != nil {
		return err
	}
	d, err := apply.NewDestroyerBuilder().
		WithFactory(r.factory).
		WithInventoryClient(invClient).
		Build()
	if err != nil {
		return err
	}

	// Always enable status events for the table printer
	if r.output == printers.TablePrinter {
		r.printStatusEvents = true
	}

	// Run the destroyer. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	ch := d.Run(ctx, inv, apply.DestroyerOptions{
		DeleteTimeout:           r.deleteTimeout,
		DeletePropagationPolicy: deletePropPolicy,
		InventoryPolicy:         inventoryPolicy,
		EmitStatusEvents:        r.printStatusEvents,
	})

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(r.output, r.ioStreams)
	return printer.Print(ch, common.DryRunNone, r.printStatusEvents)
}

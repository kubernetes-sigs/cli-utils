// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package preview

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
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/printers"
)

var (
	noPrune        = false
	previewDestroy = false
)

// GetRunner creates and returns the Runner which stores the cobra command.
func GetRunner(factory cmdutil.Factory, invFactory inventory.ClientFactory,
	loader manifestreader.ManifestLoader, ioStreams genericclioptions.IOStreams) *Runner {
	r := &Runner{
		factory:    factory,
		invFactory: invFactory,
		loader:     loader,
		ioStreams:  ioStreams,
	}
	cmd := &cobra.Command{
		Use:                   "preview (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Preview the apply of a configuration"),
		Args:                  cobra.MaximumNArgs(1),
		RunE:                  r.RunE,
	}

	cmd.Flags().BoolVar(&noPrune, "no-prune", noPrune, "If true, do not prune previously applied objects.")
	cmd.Flags().BoolVar(&r.serverSideOptions.ServerSideApply, "server-side", false,
		"If true, preview runs in the server instead of the client.")
	cmd.Flags().BoolVar(&r.serverSideOptions.ForceConflicts, "force-conflicts", false,
		"If true during server-side preview, do not report field conflicts.")
	cmd.Flags().StringVar(&r.serverSideOptions.FieldManager, "field-manager", common.DefaultFieldManager,
		"If true during server-side preview, sets field owner.")
	cmd.Flags().BoolVar(&previewDestroy, "destroy", previewDestroy, "If true, preview of destroy operations will be displayed.")
	cmd.Flags().StringVar(&r.output, "output", printers.DefaultPrinter(),
		fmt.Sprintf("Output format, must be one of %s", strings.Join(printers.SupportedPrinters(), ",")))
	cmd.Flags().StringVar(&r.inventoryPolicy, flagutils.InventoryPolicyFlag, flagutils.InventoryPolicyStrict,
		"It determines the behavior when the resources don't belong to current inventory. Available options "+
			fmt.Sprintf("%q, %q and %q.", flagutils.InventoryPolicyStrict, flagutils.InventoryPolicyAdopt, flagutils.InventoryPolicyForceAdopt))
	cmd.Flags().DurationVar(&r.timeout, "timeout", 0,
		"How long to wait before exiting")

	r.Command = cmd
	return r
}

// Command creates the Runner, returning the cobra command associated with it.
func Command(f cmdutil.Factory, invFactory inventory.ClientFactory, loader manifestreader.ManifestLoader,
	ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetRunner(f, invFactory, loader, ioStreams).Command
}

// Runner encapsulates data necessary to run the preview command.
type Runner struct {
	Command    *cobra.Command
	factory    cmdutil.Factory
	invFactory inventory.ClientFactory
	loader     manifestreader.ManifestLoader
	ioStreams  genericclioptions.IOStreams

	serverSideOptions common.ServerSideOptions
	output            string
	inventoryPolicy   string
	timeout           time.Duration
}

// RunE is the function run from the cobra command.
func (r *Runner) RunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// If specified, cancel with timeout.
	if r.timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	var ch <-chan event.Event

	drs := common.DryRunClient
	if r.serverSideOptions.ServerSideApply {
		drs = common.DryRunServer
	}

	inventoryPolicy, err := flagutils.ConvertInventoryPolicy(r.inventoryPolicy)
	if err != nil {
		return err
	}

	reader, err := r.loader.ManifestReader(cmd.InOrStdin(), flagutils.PathFromArgs(args))
	if err != nil {
		return err
	}

	if found := printers.ValidatePrinterType(r.output); !found {
		return fmt.Errorf("unknown output type %q", r.output)
	}

	objs, err := reader.Read()
	if err != nil {
		return err
	}

	invObj, objs, err := inventory.SplitUnstructureds(objs)
	if err != nil {
		return err
	}
	inv := inventory.InfoFromObject(invObj)

	invClient, err := r.invFactory.NewClient(r.factory)
	if err != nil {
		return err
	}

	// if destroy flag is set in preview, transmit it to destroyer DryRunStrategy flag
	// and pivot execution to destroy with dry-run
	if !previewDestroy {
		_, err = common.DemandOneDirectory(args)
		if err != nil {
			return err
		}
		a, err := apply.NewApplierBuilder().
			WithFactory(r.factory).
			WithInventoryClient(invClient).
			Build()
		if err != nil {
			return err
		}

		// Run the applier. It will return a channel where we can receive updates
		// to keep track of progress and any issues.
		ch = a.Run(ctx, inv, objs, apply.ApplierOptions{
			EmitStatusEvents:  false,
			NoPrune:           noPrune,
			DryRunStrategy:    drs,
			ServerSideOptions: r.serverSideOptions,
			InventoryPolicy:   inventoryPolicy,
		})
	} else {
		d, err := apply.NewDestroyerBuilder().
			WithFactory(r.factory).
			WithInventoryClient(invClient).
			Build()
		if err != nil {
			return err
		}
		ch = d.Run(ctx, inv, apply.DestroyerOptions{
			InventoryPolicy: inventoryPolicy,
			DryRunStrategy:  drs,
		})
	}

	// Print the preview strategy unless the output format is json.
	if r.output != printers.JSONPrinter {
		if drs.ServerDryRun() {
			fmt.Println("Preview strategy: server")
		} else {
			fmt.Println("Preview strategy: client")
		}
	}

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(r.output, r.ioStreams)
	return printer.Print(ch, drs, false) // Do not print status
}

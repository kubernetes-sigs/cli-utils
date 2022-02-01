// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

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

func GetApplyRunner(factory cmdutil.Factory, invFactory inventory.InventoryClientFactory,
	loader manifestreader.ManifestLoader, ioStreams genericclioptions.IOStreams) *ApplyRunner {
	r := &ApplyRunner{
		ioStreams:  ioStreams,
		factory:    factory,
		invFactory: invFactory,
		loader:     loader,
	}
	cmd := &cobra.Command{
		Use:                   "apply (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Apply a configuration to a resource by package directory or stdin"),
		RunE:                  r.RunE,
	}

	cmd.Flags().BoolVar(&r.serverSideOptions.ServerSideApply, "server-side", false,
		"If true, apply merge patch is calculated on API server instead of client.")
	cmd.Flags().BoolVar(&r.serverSideOptions.ForceConflicts, "force-conflicts", false,
		"If true, overwrite applied fields on server if field manager conflict.")
	cmd.Flags().StringVar(&r.serverSideOptions.FieldManager, "field-manager", common.DefaultFieldManager,
		"The client owner of the fields being applied on the server-side.")

	cmd.Flags().StringVar(&r.output, "output", printers.DefaultPrinter(),
		fmt.Sprintf("Output format, must be one of %s", strings.Join(printers.SupportedPrinters(), ",")))
	cmd.Flags().DurationVar(&r.period, "poll-period", 2*time.Second,
		"Polling period for resource statuses.")
	cmd.Flags().DurationVar(&r.reconcileTimeout, "reconcile-timeout", time.Duration(0),
		"Timeout threshold for waiting for all resources to reach the Current status.")
	cmd.Flags().BoolVar(&r.noPrune, "no-prune", r.noPrune,
		"If true, do not prune previously applied objects.")
	cmd.Flags().StringVar(&r.prunePropagationPolicy, "prune-propagation-policy",
		"Background", "Propagation policy for pruning")
	cmd.Flags().DurationVar(&r.pruneTimeout, "prune-timeout", time.Duration(0),
		"Timeout threshold for waiting for all pruned resources to be deleted")
	cmd.Flags().StringVar(&r.inventoryPolicy, flagutils.InventoryPolicyFlag, flagutils.InventoryPolicyStrict,
		"It determines the behavior when the resources don't belong to current inventory. Available options "+
			fmt.Sprintf("%q, %q and %q.", flagutils.InventoryPolicyStrict, flagutils.InventoryPolicyAdopt, flagutils.InventoryPolicyForceAdopt))
	cmd.Flags().DurationVar(&r.timeout, "timeout", 0,
		"How long to wait before exiting")
	cmd.Flags().BoolVar(&r.printStatusEvents, "status-events", false,
		"Print status events (always enabled for table output)")

	r.Command = cmd
	return r
}

func ApplyCommand(f cmdutil.Factory, invFactory inventory.InventoryClientFactory, loader manifestreader.ManifestLoader,
	ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetApplyRunner(f, invFactory, loader, ioStreams).Command
}

type ApplyRunner struct {
	Command    *cobra.Command
	ioStreams  genericclioptions.IOStreams
	factory    cmdutil.Factory
	invFactory inventory.InventoryClientFactory
	loader     manifestreader.ManifestLoader

	serverSideOptions      common.ServerSideOptions
	output                 string
	period                 time.Duration
	reconcileTimeout       time.Duration
	noPrune                bool
	prunePropagationPolicy string
	pruneTimeout           time.Duration
	inventoryPolicy        string
	timeout                time.Duration
	printStatusEvents      bool
}

func (r *ApplyRunner) RunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	// If specified, cancel with timeout.
	if r.timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	prunePropPolicy, err := flagutils.ConvertPropagationPolicy(r.prunePropagationPolicy)
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

	// TODO: Fix DemandOneDirectory to no longer return FileNameFlags
	// since we are no longer using them.
	_, err = common.DemandOneDirectory(args)
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

	invObj, objs, err := inventory.SplitUnstructureds(objs)
	if err != nil {
		return err
	}
	inv := inventory.WrapInventoryInfoObj(invObj)

	invClient, err := r.invFactory.NewInventoryClient(r.factory)
	if err != nil {
		return err
	}

	// Run the applier. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	a, err := apply.NewApplierBuilder().
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

	ch := a.Run(ctx, inv, objs, apply.ApplierOptions{
		ServerSideOptions: r.serverSideOptions,
		PollInterval:      r.period,
		ReconcileTimeout:  r.reconcileTimeout,
		// If we are not waiting for status, tell the applier to not
		// emit the events.
		EmitStatusEvents:       r.printStatusEvents,
		NoPrune:                r.noPrune,
		DryRunStrategy:         common.DryRunNone,
		PrunePropagationPolicy: prunePropPolicy,
		PruneTimeout:           r.pruneTimeout,
		InventoryPolicy:        inventoryPolicy,
	})

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(r.output, r.ioStreams)
	return printer.Print(ch, common.DryRunNone, r.printStatusEvents)
}

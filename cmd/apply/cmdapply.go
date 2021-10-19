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
	"sigs.k8s.io/cli-utils/cmd/printers"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/util/factory"
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

	r.Command = cmd
	return r
}

func ApplyCommand(f cmdutil.Factory, invFactory inventory.InventoryClientFactory, loader manifestreader.ManifestLoader,
	ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetApplyRunner(f, invFactory, loader, ioStreams).Command
}

type ApplyRunner struct {
	Command    *cobra.Command
	PreProcess func(info inventory.InventoryInfo, strategy common.DryRunStrategy) (inventory.InventoryPolicy, error)
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

	// Only print status events if we are waiting for status.
	//TODO: This is not the right way to do this. There are situations where
	// we do need status events event if we are not waiting for status. The
	// printers should be updated to handle this.
	var printStatusEvents bool
	if r.reconcileTimeout != time.Duration(0) || r.pruneTimeout != time.Duration(0) {
		printStatusEvents = true
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

	inv, objs, err := r.loader.InventoryInfo(objs)
	if err != nil {
		return err
	}

	if r.PreProcess != nil {
		inventoryPolicy, err = r.PreProcess(inv, common.DryRunNone)
		if err != nil {
			return err
		}
	}
	statusPoller, err := factory.NewStatusPoller(r.factory)
	if err != nil {
		return err
	}
	invClient, err := r.invFactory.NewInventoryClient(r.factory)
	if err != nil {
		return err
	}

	// Run the applier. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	a, err := apply.NewApplier(r.factory, invClient, statusPoller)
	if err != nil {
		return err
	}
	ch := a.Run(ctx, inv, objs, apply.Options{
		ServerSideOptions: r.serverSideOptions,
		PollInterval:      r.period,
		ReconcileTimeout:  r.reconcileTimeout,
		// If we are not waiting for status, tell the applier to not
		// emit the events.
		EmitStatusEvents:       printStatusEvents,
		NoPrune:                r.noPrune,
		DryRunStrategy:         common.DryRunNone,
		PrunePropagationPolicy: prunePropPolicy,
		PruneTimeout:           r.pruneTimeout,
		InventoryPolicy:        inventoryPolicy,
	})

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(r.output, r.ioStreams)
	return printer.Print(ch, common.DryRunNone, printStatusEvents)
}

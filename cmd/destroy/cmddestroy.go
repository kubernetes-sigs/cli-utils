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
	"sigs.k8s.io/cli-utils/pkg/util/factory"
)

// GetDestroyRunner creates and returns the DestroyRunner which stores the cobra command.
func GetDestroyRunner(factory cmdutil.Factory, invFactory inventory.InventoryClientFactory,
	loader manifestreader.ManifestLoader, ioStreams genericclioptions.IOStreams) *DestroyRunner {
	r := &DestroyRunner{
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

	r.Command = cmd
	return r
}

// DestroyCommand creates the DestroyRunner, returning the cobra command associated with it.
func DestroyCommand(f cmdutil.Factory, invFactory inventory.InventoryClientFactory, loader manifestreader.ManifestLoader,
	ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetDestroyRunner(f, invFactory, loader, ioStreams).Command
}

// DestroyRunner encapsulates data necessary to run the destroy command.
type DestroyRunner struct {
	Command    *cobra.Command
	PreProcess func(info inventory.InventoryInfo, strategy common.DryRunStrategy) (inventory.InventoryPolicy, error)
	ioStreams  genericclioptions.IOStreams
	factory    cmdutil.Factory
	invFactory inventory.InventoryClientFactory
	loader     manifestreader.ManifestLoader

	output                  string
	deleteTimeout           time.Duration
	deletePropagationPolicy string
	inventoryPolicy         string
	timeout                 time.Duration
}

func (r *DestroyRunner) RunE(cmd *cobra.Command, args []string) error {
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
	// Retrieve the inventory object.
	reader, err := r.loader.ManifestReader(cmd.InOrStdin(), flagutils.PathFromArgs(args))
	if err != nil {
		return err
	}
	objs, err := reader.Read()
	if err != nil {
		return err
	}
	inv, _, err := r.loader.InventoryInfo(objs)
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
	d, err := apply.NewDestroyer(r.factory, invClient, statusPoller)
	if err != nil {
		return err
	}
	// Run the destroyer. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	printStatusEvents := r.deleteTimeout != time.Duration(0)
	ch := d.Run(ctx, inv, apply.DestroyerOptions{
		DeleteTimeout:           r.deleteTimeout,
		DeletePropagationPolicy: deletePropPolicy,
		InventoryPolicy:         inventoryPolicy,
		EmitStatusEvents:        printStatusEvents,
	})

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(r.output, r.ioStreams)
	return printer.Print(ch, common.DryRunNone, printStatusEvents)
}

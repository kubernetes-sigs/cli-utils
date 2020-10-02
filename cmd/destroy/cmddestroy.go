// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package destroy

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/cmd/printers"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/provider"
)

// GetDestroyRunner creates and returns the DestroyRunner which stores the cobra command.
func GetDestroyRunner(provider provider.Provider, ioStreams genericclioptions.IOStreams) *DestroyRunner {
	r := &DestroyRunner{
		Destroyer: apply.NewDestroyer(provider, ioStreams),
		ioStreams: ioStreams,
		provider:  provider,
	}
	cmd := &cobra.Command{
		Use:                   "destroy (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Destroy all the resources related to configuration"),
		RunE:                  r.RunE,
	}

	r.Command = cmd
	return r
}

// DestroyCommand creates the DestroyRunner, returning the cobra command associated with it.
func DestroyCommand(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	provider := provider.NewProvider(f, inventory.WrapInventoryObj)
	return GetDestroyRunner(provider, ioStreams).Command
}

// DestroyRunner encapsulates data necessary to run the destroy command.
type DestroyRunner struct {
	Command   *cobra.Command
	ioStreams genericclioptions.IOStreams
	Destroyer *apply.Destroyer
	provider  provider.Provider
}

func (r *DestroyRunner) RunE(cmd *cobra.Command, args []string) error {
	err := r.Destroyer.Initialize(cmd, args)
	if err != nil {
		return err
	}

	// Retrieve the inventory object.
	reader, err := r.provider.ManifestReader(cmd.InOrStdin(), args)
	if err != nil {
		return err
	}
	infos, err := reader.Read()
	if err != nil {
		return err
	}
	inv, _, err := inventory.SplitInfos(infos)
	if err != nil {
		return err
	}

	// Run the destroyer. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	ch := r.Destroyer.Run(inv)

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(printers.EventsPrinter, r.ioStreams)
	return printer.Print(ch, r.Destroyer.DryRunStrategy)
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package destroy

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/pkg/apply"
)

// GetDestroyRunner creates and returns the DestroyRunner which stores the cobra command.
func GetDestroyRunner(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *DestroyRunner {
	r := &DestroyRunner{
		Destroyer: apply.NewDestroyer(f, ioStreams),
		ioStreams: ioStreams,
	}
	cmd := &cobra.Command{
		Use:                   "destroy (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Destroy all the resources related to configuration"),
		RunE:                  r.RunE,
	}

	r.Destroyer.SetFlags(cmd)

	// The following flags are added, but hidden because other code
	// dependencies when parsing flags. These flags are hidden and unused.
	var unusedBool bool
	cmd.Flags().BoolVar(&unusedBool, "dry-run", unusedBool, "NOT USED")
	cmdutil.AddValidateFlags(cmd)
	_ = cmd.Flags().MarkHidden("validate")
	// Server-side flags are hidden for now.
	cmdutil.AddServerSideApplyFlags(cmd)
	_ = cmd.Flags().MarkHidden("server-side")
	_ = cmd.Flags().MarkHidden("force-conflicts")
	_ = cmd.Flags().MarkHidden("field-manager")

	r.Command = cmd
	return r
}

// DestroyCommand creates the DestroyRunner, returning the cobra command associated with it.
func DestroyCommand(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetDestroyRunner(f, ioStreams).Command
}

// DestroyRunner encapsulates data necessary to run the destroy command.
type DestroyRunner struct {
	Command   *cobra.Command
	ioStreams genericclioptions.IOStreams
	Destroyer *apply.Destroyer
}

func (r *DestroyRunner) RunE(cmd *cobra.Command, args []string) error {
	err := r.Destroyer.Initialize(cmd, args)
	if err != nil {
		return err
	}

	// Run the destroyer. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	ch := r.Destroyer.Run()

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := &apply.BasicPrinter{
		IOStreams: r.ioStreams,
	}
	return printer.Print(ch, r.Destroyer.DryRunStrategy)
}

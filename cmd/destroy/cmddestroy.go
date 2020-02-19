// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package destroy

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/pkg/apply"
)

// NewCmdDestroy creates the `destroy` command
func NewCmdDestroy(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	destroyer := apply.NewDestroyer(f, ioStreams)
	printer := &apply.BasicPrinter{
		IOStreams: ioStreams,
	}

	cmd := &cobra.Command{
		Use:                   "destroy (FILENAME... | DIRECTORY)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Destroy all the resources related to configuration"),
		Args:                  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			paths := args
			cmdutil.CheckErr(destroyer.Initialize(cmd, paths))

			// Run the destroyer. It will return a channel where we can receive updates
			// to keep track of progress and any issues.
			ch := destroyer.Run()

			// The printer will print updates from the channel. It will block
			// until the channel is closed.
			printer.Print(ch)
		},
	}

	cmdutil.CheckErr(destroyer.SetFlags(cmd))

	cmdutil.AddValidateFlags(cmd)
	_ = cmd.Flags().MarkHidden("validate")

	cmd.Flags().BoolVar(&destroyer.DryRun, "dry-run", destroyer.DryRun, "NOT USED")
	_ = cmd.Flags().MarkHidden("dry-run")

	cmdutil.AddServerSideApplyFlags(cmd)
	return cmd
}

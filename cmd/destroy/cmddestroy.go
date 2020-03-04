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
		Use:                   "destroy (DIRECTORY)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Destroy all the resources related to configuration"),
		Run: func(cmd *cobra.Command, args []string) {
			paths := args
			cmdutil.CheckErr(destroyer.Initialize(cmd, paths))

			// Run the destroyer. It will return a channel where we can receive updates
			// to keep track of progress and any issues.
			ch := destroyer.Run()

			// The printer will print updates from the channel. It will block
			// until the channel is closed.
			printer.Print(ch, false)
		},
	}

	cmdutil.CheckErr(destroyer.SetFlags(cmd))

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

	return cmd
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/pkg/apply"
)

// NewCmdApply creates the `apply` command
func NewCmdApply(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	applier := apply.NewApplier(f, ioStreams)
	printer := &apply.BasicPrinter{
		IOStreams: ioStreams,
	}

	cmd := &cobra.Command{
		Use:                   "apply DIRECTORY",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Apply a configuration to a resource by filename or stdin"),
		Run: func(cmd *cobra.Command, args []string) {
			paths := args
			cmdutil.CheckErr(applier.Initialize(cmd, paths))

			// Create a context with the provided timout from the cobra parameter.
			ctx, cancel := context.WithTimeout(context.Background(), applier.StatusOptions.Timeout)
			defer cancel()
			// Run the applier. It will return a channel where we can receive updates
			// to keep track of progress and any issues.
			ch := applier.Run(ctx)

			// The printer will print updates from the channel. It will block
			// until the channel is closed.
			printer.Print(ch, false)
		},
	}

	cmd.Flags().BoolVar(&applier.NoPrune, "no-prune", applier.NoPrune, "If true, do not prune previously applied objects.")
	cmdutil.CheckErr(applier.SetFlags(cmd))

	// The following flags are added, but hidden because other code
	// depend on them when parsing flags. These flags are hidden and unused.
	var unusedBool bool
	cmd.Flags().BoolVar(&unusedBool, "dry-run", unusedBool, "NOT USED")
	_ = cmd.Flags().MarkHidden("dry-run")
	cmdutil.AddValidateFlags(cmd)
	_ = cmd.Flags().MarkHidden("validate")
	// Server-side flags are hidden for now.
	cmdutil.AddServerSideApplyFlags(cmd)
	_ = cmd.Flags().MarkHidden("server-side")
	_ = cmd.Flags().MarkHidden("force-conflicts")
	_ = cmd.Flags().MarkHidden("field-manager")

	return cmd
}

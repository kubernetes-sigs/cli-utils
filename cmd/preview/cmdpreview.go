// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package preview

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
func NewCmdPreview(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	applier := apply.NewApplier(f, ioStreams)
	printer := &apply.BasicPrinter{
		IOStreams: ioStreams,
	}

	cmd := &cobra.Command{
		Use:                   "preview (-f FILENAME | -k DIRECTORY)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Preview the apply of a configuration"),
		Args:                  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				// check is kustomize, if so update
				applier.ApplyOptions.DeleteFlags.FileNameFlags.Kustomize = &args[0]
			}

			// Set DryRun option true before Initialize. DryRun is propagated to
			// ApplyOptions and PruneOptions in Initialize.
			applier.DryRun = true
			cmdutil.CheckErr(applier.Initialize(cmd))

			// Create a context with the provided timout from the cobra parameter.
			ctx, cancel := context.WithTimeout(context.Background(), applier.StatusOptions.Timeout)
			defer cancel()
			// Run the applier. It will return a channel where we can receive updates
			// to keep track of progress and any issues.
			ch := applier.Run(ctx)

			// The printer will print updates from the channel. It will block
			// until the channel is closed.
			printer.Print(ch)
		},
	}

	applier.SetFlags(cmd)

	cmdutil.AddValidateFlags(cmd)
	cmd.Flags().BoolVar(&applier.NoPrune, "no-prune", applier.NoPrune, "If true, do not prune previously applied objects.")
	// Necessary because ApplyOptions depends on it--not used.
	cmd.Flags().BoolVar(&applier.DryRun, "dry-run", applier.DryRun, "NOT USED")
	_ = cmd.Flags().MarkHidden("dry-run")

	cmdutil.AddServerSideApplyFlags(cmd)

	return cmd
}

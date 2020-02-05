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
		Use:                   "apply (-f FILENAME | -k DIRECTORY)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Apply a configuration to a resource by filename or stdin"),
		Args:                  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				// check is kustomize, if so update
				applier.ApplyOptions.DeleteFlags.FileNameFlags.Kustomize = &args[0]
			}

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
	cmd.Flags().BoolVar(&applier.DryRun, "dry-run", applier.DryRun,
		"If true, only print the object that would be sent, without sending it. Warning: --dry-run cannot accurately output the result "+
			"of merging the local manifest and the server-side data. Use --server-dry-run to get the merged result instead.")
	cmd.Flags().BoolVar(&applier.ApplyOptions.ServerDryRun, "server-dry-run", applier.ApplyOptions.ServerDryRun,
		"If true, request will be sent to server with dry-run flag, which means the modifications won't be persisted. This is an alpha feature and flag.")
	cmdutil.AddServerSideApplyFlags(cmd)

	return cmd
}

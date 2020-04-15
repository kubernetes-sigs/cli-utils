// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package preview

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
)

// NewCmdPreview creates the `preview` command
func NewCmdPreview(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	applier := apply.NewApplier(f, ioStreams)
	destroyer := apply.NewDestroyer(f, ioStreams)

	printer := &apply.BasicPrinter{
		IOStreams: ioStreams,
	}

	cmd := &cobra.Command{
		Use:                   "preview DIRECTORY",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Preview the apply of a configuration"),
		Args:                  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var ch <-chan event.Event
			cmdutil.CheckErr(destroyer.Initialize(cmd, args))
			// if destroy flag is set in preview, transmit it to destroyer DryRun flag
			// and pivot execution to destroy with dry-run
			if !destroyer.DryRun {
				// Set DryRun option true before Initialize. DryRun is propagated to
				// ApplyOptions and PruneOptions in Initialize.
				applier.DryRun = true
				cmdutil.CheckErr(applier.Initialize(cmd, args))

				// Create a context with the provided timout from the cobra parameter.
				ctx, cancel := context.WithTimeout(context.Background(), applier.StatusOptions.Timeout)
				defer cancel()

				// Run the applier. It will return a channel where we can receive updates
				// to keep track of progress and any issues.
				ch = applier.Run(ctx)
			} else {
				ch = destroyer.Run()
			}

			// The printer will print updates from the channel. It will block
			// until the channel is closed.
			printer.Print(ch, true)
		},
	}

	cmd.Flags().BoolVar(&applier.NoPrune, "no-prune", applier.NoPrune, "If true, do not prune previously applied objects.")
	cmdutil.CheckErr(applier.SetFlags(cmd))

	// The following flags are added, but hidden because other code
	// dependend on them when parsing flags. These flags are hidden and unused.
	var unusedBool bool
	cmd.Flags().BoolVar(&unusedBool, "dry-run", unusedBool, "NOT USED")
	cmd.Flags().BoolVar(&destroyer.DryRun, "destroy", destroyer.DryRun, "If true, preview of destroy operations will be displayed.")
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

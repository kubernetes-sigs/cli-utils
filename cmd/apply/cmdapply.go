// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/cmd/printers"
	"sigs.k8s.io/cli-utils/pkg/apply"
)

func GetApplyRunner(f util.Factory, ioStreams genericclioptions.IOStreams) *ApplyRunner {
	r := &ApplyRunner{
		applier:   apply.NewApplier(f, ioStreams),
		ioStreams: ioStreams,
	}
	cmd := &cobra.Command{
		Use:                   "apply DIRECTORY",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Apply a configuration to a resource by filename or stdin"),
		Run:                   r.Run,
	}

	cmd.Flags().BoolVar(&r.applier.NoPrune, "no-prune", r.applier.NoPrune, "If true, do not prune previously applied objects.")
	cmdutil.CheckErr(r.applier.SetFlags(cmd))

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

	cmd.Flags().StringVar(&r.output, "output", printers.DefaultPrinter(),
		fmt.Sprintf("Output format, must be one of %s", strings.Join(printers.SupportedPrinters(), ",")))

	r.command = cmd
	return r
}

func ApplyCommand(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetApplyRunner(f, ioStreams).command
}

type ApplyRunner struct {
	command   *cobra.Command
	ioStreams genericclioptions.IOStreams
	applier   *apply.Applier

	output string
}

func (r *ApplyRunner) Run(cmd *cobra.Command, args []string) {
	cmdutil.CheckErr(r.applier.Initialize(cmd, args))

	// Run the applier. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	ch := r.applier.Run(context.Background())

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(r.output, r.ioStreams)
	printer.Print(ch, false)
}

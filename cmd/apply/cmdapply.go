// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/cmd/printers"
	"sigs.k8s.io/cli-utils/pkg/apply"
)

func GetApplyRunner(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *ApplyRunner {
	r := &ApplyRunner{
		applier:   apply.NewApplier(f, ioStreams),
		ioStreams: ioStreams,
	}
	cmd := &cobra.Command{
		Use:                   "apply (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Apply a configuration to a resource by package directory or stdin"),
		RunE:                  r.RunE,
	}

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

	cmd.Flags().BoolVar(&r.wait, "wait-for-reconcile", false,
		"Wait for all applied resources to reach the Current status.")
	cmd.Flags().DurationVar(&r.period, "wait-polling-period", 2*time.Second,
		"Polling period for resource statuses.")
	cmd.Flags().DurationVar(&r.timeout, "wait-timeout", time.Minute,
		"Timeout threshold for waiting for all resources to reach the Current status.")
	cmd.Flags().BoolVar(&r.noPrune, "no-prune", r.noPrune,
		"If true, do not prune previously applied objects.")
	cmd.Flags().StringVar(&r.prunePropagationPolicy, "prune-propagation-policy",
		"Background", "Propagation policy for pruning")

	r.command = cmd
	return r
}

func ApplyCommand(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetApplyRunner(f, ioStreams).command
}

type ApplyRunner struct {
	command   *cobra.Command
	ioStreams genericclioptions.IOStreams
	applier   *apply.Applier

	output                 string
	wait                   bool
	period                 time.Duration
	timeout                time.Duration
	noPrune                bool
	prunePropagationPolicy string
}

func (r *ApplyRunner) RunE(cmd *cobra.Command, args []string) error {
	prunePropPolicy, err := convertPropagationPolicy(r.prunePropagationPolicy)
	if err != nil {
		return err
	}

	cmdutil.CheckErr(r.applier.Initialize(cmd, args))

	// Run the applier. It will return a channel where we can receive updates
	// to keep track of progress and any issues.
	ch := r.applier.Run(context.Background(), apply.Options{
		WaitForReconcile: r.wait,
		PollInterval:     r.period,
		WaitTimeout:      r.timeout,
		// If we are not waiting for status, tell the applier to not
		// emit the events.
		EmitStatusEvents:       r.wait,
		NoPrune:                r.noPrune,
		DryRun:                 false,
		PrunePropagationPolicy: prunePropPolicy,
	})

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := printers.GetPrinter(r.output, r.ioStreams)
	printer.Print(ch, false)
	return nil
}

// convertPropagationPolicy converts a propagationPolicy described as a
// string to a DeletionPropagation type that is passed into the Applier.
func convertPropagationPolicy(propagationPolicy string) (metav1.DeletionPropagation, error) {
	switch propagationPolicy {
	case string(metav1.DeletePropagationForeground):
		return metav1.DeletePropagationForeground, nil
	case string(metav1.DeletePropagationBackground):
		return metav1.DeletePropagationBackground, nil
	case string(metav1.DeletePropagationOrphan):
		return metav1.DeletePropagationOrphan, nil
	default:
		return metav1.DeletePropagationBackground, fmt.Errorf(
			"prune propagation policy must be one of Background, Foreground, Orphan")
	}
}

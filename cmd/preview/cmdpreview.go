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
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/provider"
	"sigs.k8s.io/kustomize/kyaml/setters2"
)

var (
	noPrune        = false
	serverDryRun   = false
	previewDestroy = false
)

// GetPreviewRunner creates and returns the PreviewRunner which stores the cobra command.
func GetPreviewRunner(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *PreviewRunner {
	provider := provider.NewProvider(f, inventory.WrapInventoryObj)
	r := &PreviewRunner{
		Applier:   apply.NewApplier(provider, ioStreams),
		Destroyer: apply.NewDestroyer(provider, ioStreams),
		ioStreams: ioStreams,
		provider:  provider,
	}
	cmd := &cobra.Command{
		Use:                   "preview (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Preview the apply of a configuration"),
		Args:                  cobra.MaximumNArgs(1),
		RunE:                  r.RunE,
	}

	r.Applier.SetFlags(cmd)

	cmd.Flags().BoolVar(&noPrune, "no-prune", noPrune, "If true, do not prune previously applied objects.")
	cmd.Flags().BoolVar(&serverDryRun, "server-side", serverDryRun, "If true, preview runs in the server instead of the client.")

	// The following flags are added, but hidden because other code
	// dependend on them when parsing flags. These flags are hidden and unused.
	var unusedBool bool
	cmd.Flags().BoolVar(&unusedBool, "dry-run", unusedBool, "NOT USED")
	cmd.Flags().BoolVar(&previewDestroy, "destroy", previewDestroy, "If true, preview of destroy operations will be displayed.")
	_ = cmd.Flags().MarkHidden("dry-run")
	cmdutil.AddValidateFlags(cmd)
	_ = cmd.Flags().MarkHidden("validate")
	cmd.Flags().Bool("force-conflicts", false, "If true, server-side apply will force the changes against conflicts.")
	cmd.Flags().String("field-manager", "kubectl", "Name of the manager used to track field ownership.")
	// hide unwanted server-side flags
	_ = cmd.Flags().MarkHidden("force-conflicts")
	_ = cmd.Flags().MarkHidden("field-manager")

	r.Command = cmd
	return r
}

// PreviewCommand creates the PreviewRunner, returning the cobra command associated with it.
func PreviewCommand(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	return GetPreviewRunner(f, ioStreams).Command
}

// PreviewRunner encapsulates data necessary to run the preview command.
type PreviewRunner struct {
	Command   *cobra.Command
	ioStreams genericclioptions.IOStreams
	Applier   *apply.Applier
	Destroyer *apply.Destroyer
	provider  provider.Provider
}

// RunE is the function run from the cobra command.
func (r *PreviewRunner) RunE(cmd *cobra.Command, args []string) error {
	err := setters2.CheckRequiredSettersSet()
	if err != nil {
		return err
	}
	var ch <-chan event.Event
	err = r.Destroyer.Initialize(cmd, args)
	if err != nil {
		return err
	}

	drs := common.DryRunClient
	if serverDryRun {
		drs = common.DryRunServer
	}

	if previewDestroy {
		r.Destroyer.DryRunStrategy = drs
	}

	// if destroy flag is set in preview, transmit it to destroyer DryRunStrategy flag
	// and pivot execution to destroy with dry-run
	if !r.Destroyer.DryRunStrategy.ClientOrServerDryRun() {
		err = r.Applier.Initialize(cmd)
		if err != nil {
			return err
		}

		// Create a context
		ctx := context.Background()

		_, err := common.DemandOneDirectory(args)
		if err != nil {
			return err
		}

		// Fetch the namespace from the configloader. The source of this
		// either the namespace flag or the context. If the namespace is provided
		// with the flag, enforceNamespace will be true. In this case, it is
		// an error if any of the resources in the package has a different
		// namespace set.
		namespace, enforceNamespace, err := r.provider.Factory().ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return err
		}

		var reader manifestreader.ManifestReader
		readerOptions := manifestreader.ReaderOptions{
			Factory:          r.provider.Factory(),
			Namespace:        namespace,
			EnforceNamespace: enforceNamespace,
		}
		if len(args) == 0 {
			reader = &manifestreader.StreamManifestReader{
				ReaderName:    "stdin",
				Reader:        cmd.InOrStdin(),
				ReaderOptions: readerOptions,
			}
		} else {
			reader = &manifestreader.PathManifestReader{
				Path:          args[0],
				ReaderOptions: readerOptions,
			}
		}
		infos, err := reader.Read()
		if err != nil {
			return err
		}

		// Run the applier. It will return a channel where we can receive updates
		// to keep track of progress and any issues.
		ch = r.Applier.Run(ctx, infos, apply.Options{
			EmitStatusEvents: false,
			NoPrune:          noPrune,
			DryRunStrategy:   drs,
		})
	} else {
		ch = r.Destroyer.Run()
	}

	// The printer will print updates from the channel. It will block
	// until the channel is closed.
	printer := &apply.BasicPrinter{
		IOStreams: r.ioStreams,
	}
	return printer.Print(ch, drs)
}

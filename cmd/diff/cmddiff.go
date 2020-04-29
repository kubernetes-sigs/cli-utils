// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package diff

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/diff"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/pkg/common"
)

// NewCmdDiff returns cobra command to implement client-side diff of package
// directory. For each local config file, get the resource in the cluster
// and diff the local config resource against the resource in the cluster.
func NewCmdDiff(f util.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	options := diff.NewDiffOptions(ioStreams)
	cmd := &cobra.Command{
		Use:                   "diff (DIRECTORY | STDIN)",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Diff local config against cluster applied version"),
		Args:                  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(Initialize(options, f, args))
			util.CheckErr(options.Run())
		},
	}

	return cmd
}

// Initialize fills in the DiffOptions in preparation for DiffOptions.Run().
// Returns error if there is an error filling in the options or if there
// is not one argument that is a directory.
func Initialize(o *diff.DiffOptions, f util.Factory, args []string) error {
	// Validate the only argument is a (package) directory path.
	filenameFlags, err := common.DemandOneDirectory(args)
	if err != nil {
		return err
	}
	// We do not want to diff the inventory object. So we expand
	// the config file paths, excluding the inventory object.
	filenameFlags, err = common.ExpandPackageDir(filenameFlags)
	if err != nil {
		return err
	}
	o.FilenameOptions = filenameFlags.ToOptions()

	o.OpenAPISchema, err = f.OpenAPISchema()
	if err != nil {
		return err
	}

	o.DiscoveryClient, err = f.ToDiscoveryClient()
	if err != nil {
		return err
	}

	o.DynamicClient, err = f.DynamicClient()
	if err != nil {
		return err
	}

	o.DryRunVerifier = &apply.DryRunVerifier{
		Finder:        util.NewCRDFinder(util.CRDFromDynamic(o.DynamicClient)),
		OpenAPIGetter: o.DiscoveryClient,
	}

	o.CmdNamespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.Builder = f.NewBuilder()

	// We don't support server-side apply diffing yet.
	o.ServerSideApply = false
	o.ForceConflicts = false

	return nil
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package initcmd

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/pkg/config"
)

// NewCmdInit creates the `init` command, which generates the
// grouping object template ConfigMap for a package.
func NewCmdInit(ioStreams genericclioptions.IOStreams) *cobra.Command {
	io := config.NewInitOptions(ioStreams)
	cmd := &cobra.Command{
		Use:                   "init DIRECTORY",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Create a prune manifest ConfigMap as a grouping object"),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(io.Complete(args))
			cmdutil.CheckErr(io.Run())
		},
	}
	cmd.Flags().StringVarP(&io.GroupName, "group-name", "g", "", "Name to group applied resources. Must be composed of valid label characters.")
	return cmd
}

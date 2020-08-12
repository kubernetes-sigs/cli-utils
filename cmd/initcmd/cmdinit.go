// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package initcmd

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/util/i18n"
	"sigs.k8s.io/cli-utils/pkg/config"
)

// NewCmdInit creates the `init` command, which generates the
// inventory object template ConfigMap for a package.
func NewCmdInit(ioStreams genericclioptions.IOStreams) *cobra.Command {
	io := config.NewInitOptions(ioStreams)
	cmd := &cobra.Command{
		Use:                   "init DIRECTORY",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Create a prune manifest ConfigMap as a inventory object"),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := io.Complete(args)
			if err != nil {
				return err
			}
			return io.Run()
		},
	}
	cmd.Flags().StringVarP(&io.InventoryID, "inventory-id", "i", "", "Identifier for group of applied resources. Must be composed of valid label characters.")
	cmd.Flags().StringVarP(&io.Namespace, "inventory-namespace", "", "", "namespace for the resources to be initialized")
	return cmd
}

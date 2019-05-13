/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package delete

import (
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli"
)

// GetDeleteCommand returns the `prune` cobra Command
func GetDeleteCommand(a util.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete resources from a Kubernetes cluster.",
		Long: `Delete resources from a Kubernetes cluster.
The resource configurations can be from a Kustomization directory.
The path of the resource configurations should be passed to delete
as an argument.

	# Delete the configurations from a directory containing kustomization.yaml - e.g. dir/kustomization.yaml
	k2 delete dir
`,
		Args: cobra.MinimumNArgs(1),
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		for i := range args {
			r, err := wirecli.DoDelete(clik8s.ResourceConfigPath(args[i]), cmd.OutOrStdout(), a)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Resources: %v\n", len(r.Resources))
		}
		return nil
	}

	return cmd
}

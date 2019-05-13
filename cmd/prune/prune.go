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

package prune

import (
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli"
)

// GetPruneCommand returns the `prune` cobra Command
func GetPruneCommand(a util.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune obsolete resources.",
		Long: `Prune obsolete resources.
		# Prune the configurations from a directory containing kustomization.yaml -e.g. dir/kustomization.yaml
		k2 prune dir

The pruning is done based on checking the inventory annotation that
is stored in the resource configuration that is passed to prune command.
The inventory annotation kustomize.k8s.io/Inventory has following format:
{
  "current":
    {
      "apps_v1_Deployment|default|mysql":null,
      "~G_v1_Secret|default|pass-dfg7h97cf6":
        [
          {
            "group":"apps",
            "version":"v1",
            "kind":"Deployment",
            "name":"mysql",
            "namespace":"default",
          }
        ],
      "~G_v1_Service|default|mysql":null
    }
  }
  "previous:
      {
      "apps_v1_Deployment|default|mysql":null,
      "~G_v1_Secret|default|pass-dfg7h97cf6":
        [
          {
            "group":"apps",
            "version":"v1",
            "kind":"Deployment",
            "name":"mysql",
            "namespace":"default",
          }
        ],
      "~G_v1_Service|default|mysql":null
    }
  }
}
Any objects in the previous part that don't show up in the current part will be pruned.
For more information, see https://github.com/kubernetes-sigs/kustomize/blob/master/docs/inventory_object.md.
`,
		Args: cobra.MinimumNArgs(1),
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		for i := range args {
			r, err := wirecli.DoPrune(clik8s.ResourceConfigPath(args[i]), cmd.OutOrStdout(), a)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Resources: %v\n", len(r.Resources))
		}
		return nil
	}

	return cmd
}

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

package status

import (
	"fmt"
	//"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirestatus"
)

// GetApplyStatusCommand returns a new `apply status` command
func GetApplyStatusCommand(a util.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: ".",
		Long:  ``,
		Args:  cobra.MinimumNArgs(1),
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		for i := range args {
			result, err := wirestatus.DoStatus(clik8s.ResourceConfigPath(args[i]), cmd.OutOrStdout(), a)
			for i := range result.Resources {
				u := result.Resources[i].Resource
				fmt.Fprintf(cmd.OutOrStdout(), "%s/%s   %s", u.GetKind(), u.GetName(), result.Resources[i].Status)
				if result.Resources[i].Error != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "(err: %s)", result.Resources[i].Error)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n")
			}
			if err != nil {
				return err
			}
		}
		return nil
	}

	return cmd
}

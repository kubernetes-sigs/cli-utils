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

	"github.com/spf13/cobra"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirek8s"
)

// GetApplyStatusCommand returns a new `apply status` command
func GetApplyStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: ".",
		Long:  ``,
		Args:  cobra.MinimumNArgs(1),
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		for i := range args {
			r, err := wirecli.DoStatus(clik8s.ResourceConfigPath(args[i]), cmd.OutOrStdout())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Resources: %v\n", len(r.Resources))
		}
		return nil
	}

	// Add Flags
	wirek8s.Flags(cmd)
	return cmd
}

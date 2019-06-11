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
	"os"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/status"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wirestatus"
)

// GetApplyStatusCommand returns a new `apply status` command
func GetApplyStatusCommand(a util.Args) *cobra.Command {
	var wait bool
	var every int
	var count int
	var iteration = 0
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display status for resources",
		Long: `Display status for resources.
The resource configurations can be from a Kustomization directory.
The path of the resource configurations should be passed to apply
as an argument.
	# Get status for the configurations from a directory containing kustomization.yaml - e.g. dir/kustomization.yaml
	k2 apply status dir
`,
		Args: cobra.MinimumNArgs(1),
	}

	cmd.Flags().BoolVar(&wait, "wait", false, "bool")
	cmd.Flags().IntVar(&every, "every", 5, "check every n seconds with wait flag")
	cmd.Flags().IntVar(&count, "count", 0, "how many times to check for status with a gap of --every secs")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {

		for {
			iteration++
			ok := true
			for i := range args {
				result, err := wirestatus.DoStatus(clik8s.ResourceConfigPath(args[i]), cmd.OutOrStdout(), a)
				if err != nil {
					fmt.Fprintf(cmd.OutOrStderr(), "Err: %s\n", err)
				}
				if !status.StableOrTerminal(result.Resources) {
					ok = false
				}
			}

			if ok || !wait || (count != 0 && iteration >= count) {
				if !ok {
					os.Exit(-1)
				}
				break
			}
			time.Sleep(time.Duration(every) * time.Second)

		}
		return nil
	}

	return cmd
}

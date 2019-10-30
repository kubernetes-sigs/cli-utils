package status

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cli-utils/internal/pkg/clik8s"
	"sigs.k8s.io/cli-utils/internal/pkg/status"
	"sigs.k8s.io/cli-utils/internal/pkg/util"
	"sigs.k8s.io/cli-utils/internal/pkg/wirecli/wirestatus"
)

func GetStatusCommand(a util.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use: "status",
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		for i := range args {
			result, err := wirestatus.DoStatus(clik8s.ResourceConfigPath(args[i]), a)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "Err: %s\n", err)
			}
			printStatus(cmd.OutOrStdout(), result)
		}
		return nil
	}
	return cmd
}

func printStatus(w io.Writer, results []status.ResourceResult) {
	table := tablewriter.NewWriter(w)
	table.SetRowLine(false)
	table.SetHeader([]string{
		"GroupKind", "Namespace", "Name", "Status"})
	for _, res := range results {
		var status string
		if res.Error != nil {
			status = res.Error.Error()
		} else {
			status = res.Result.Status.String()
		}

		table.Append([]string{
			res.Resource.GroupVersionKind().GroupKind().String(),
			res.Resource.GetNamespace(),
			res.Resource.GetName(),
			status,
		})
	}
	table.Render()
}

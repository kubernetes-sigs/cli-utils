package main

import (
	"flag"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/cmd/apply"
	"sigs.k8s.io/cli-utils/cmd/diff"

	// This is here rather than in the libraries because of
	// https://github.com/kubernetes-sigs/kustomize/issues/2060
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var cmd = &cobra.Command{
	Use:   "kapply",
	Short: "Perform cluster operations using declarative configuration",
	Long:  "Perform cluster operations using declarative configuration",
}

func main() {
	// configure kubectl dependencies and flags
	flags := cmd.Flags()
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	f := util.NewFactory(matchVersionKubeConfigFlags)

	ioStreams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	names := []string{"apply", "diff"}
	applyCmd := apply.NewCmdApply(f, ioStreams)
	updateHelp(names, applyCmd)
	diffCmd := diff.NewCmdDiff(f, ioStreams)
	updateHelp(names, diffCmd)

	cmd.AddCommand(applyCmd, diffCmd)

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// updateHelp replaces `kubectl` help messaging with `kapply` help messaging
func updateHelp(names []string, c *cobra.Command) {
	for i := range names {
		name := names[i]
		c.Short = strings.ReplaceAll(c.Short, "kubectl "+name, "kapply "+name)
		c.Long = strings.ReplaceAll(c.Long, "kubectl "+name, "kapply "+name)
		c.Example = strings.ReplaceAll(c.Example, "kubectl "+name, "kapply "+name)
	}
}

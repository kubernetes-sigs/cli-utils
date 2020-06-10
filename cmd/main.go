// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/logs"
	"sigs.k8s.io/cli-utils/cmd/apply"
	"sigs.k8s.io/cli-utils/cmd/destroy"
	"sigs.k8s.io/cli-utils/cmd/diff"
	"sigs.k8s.io/cli-utils/cmd/initcmd"
	"sigs.k8s.io/cli-utils/cmd/preview"
	"sigs.k8s.io/cli-utils/cmd/status"
	"sigs.k8s.io/cli-utils/pkg/util/factory"

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
	flags := cmd.PersistentFlags()
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(&factory.CachingRESTClientGetter{
		Delegate: kubeConfigFlags,
	})
	matchVersionKubeConfigFlags.AddFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	f := util.NewFactory(matchVersionKubeConfigFlags)

	ioStreams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	names := []string{"init", "apply", "preview", "diff", "destroy", "status"}
	initCmd := initcmd.NewCmdInit(ioStreams)
	updateHelp(names, initCmd)
	applyCmd := apply.ApplyCommand(f, ioStreams)
	updateHelp(names, applyCmd)
	previewCmd := preview.NewCmdPreview(f, ioStreams)
	updateHelp(names, previewCmd)
	diffCmd := diff.NewCmdDiff(f, ioStreams)
	updateHelp(names, diffCmd)
	destroyCmd := destroy.NewCmdDestroy(f, ioStreams)
	updateHelp(names, destroyCmd)
	statusCmd := status.StatusCommand(f, ioStreams)
	updateHelp(names, statusCmd)

	cmd.AddCommand(initCmd, applyCmd, diffCmd, destroyCmd, previewCmd, statusCmd)

	logs.InitLogs()
	defer logs.FlushLogs()

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

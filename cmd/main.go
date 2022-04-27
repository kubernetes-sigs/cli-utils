// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/cmd/apply"
	"sigs.k8s.io/cli-utils/cmd/destroy"
	"sigs.k8s.io/cli-utils/cmd/diff"
	"sigs.k8s.io/cli-utils/cmd/initcmd"
	"sigs.k8s.io/cli-utils/cmd/preview"
	"sigs.k8s.io/cli-utils/cmd/status"
	"sigs.k8s.io/cli-utils/pkg/flowcontrol"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"

	// This is here rather than in the libraries because of
	// https://github.com/kubernetes-sigs/kustomize/issues/2060
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	cmd := &cobra.Command{
		Use:   "kapply",
		Short: "Perform cluster operations using declarative configuration",
		Long:  "Perform cluster operations using declarative configuration",
		// We silence error reporting from Cobra here since we want to improve
		// the error messages coming from the commands.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// configure kubectl dependencies and flags
	flags := cmd.PersistentFlags()
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(flags)
	flags.AddGoFlagSet(flag.CommandLine)
	f := util.NewFactory(matchVersionKubeConfigFlags)

	// Update ConfigFlags before subcommands run that talk to the server.
	preRunE := newConfigFilerPreRunE(f, kubeConfigFlags)

	ioStreams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	loader := manifestreader.NewManifestLoader(f)
	invFactory := inventory.ClusterClientFactory{StatusPolicy: inventory.StatusPolicyNone}

	names := []string{"init", "apply", "destroy", "diff", "preview", "status"}
	subCmds := []*cobra.Command{
		initcmd.NewCmdInit(f, ioStreams),
		apply.Command(f, invFactory, loader, ioStreams),
		destroy.Command(f, invFactory, loader, ioStreams),
		diff.NewCommand(f, ioStreams),
		preview.Command(f, invFactory, loader, ioStreams),
		status.Command(f, invFactory, loader),
	}
	for _, subCmd := range subCmds {
		subCmd.PreRunE = preRunE
		updateHelp(names, subCmd)
		cmd.AddCommand(subCmd)
	}

	code := cli.Run(cmd)
	os.Exit(code)
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

// newConfigFilerPreRunE returns a cobra command PreRunE function that
// performs a lookup to determine if server-side throttling is enabled. If so,
// client-side throttling is disabled in the ConfigFlags.
func newConfigFilerPreRunE(f util.Factory, configFlags *genericclioptions.ConfigFlags) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		restConfig, err := f.ToRESTConfig()
		if err != nil {
			return err
		}
		enabled, err := flowcontrol.IsEnabled(ctx, restConfig)
		if err != nil {
			return fmt.Errorf("checking server-side throttling enablement: %w", err)
		}
		if enabled {
			// Disable client-side throttling.
			klog.V(3).Infof("Client-side throttling disabled")
			// WrapConfigFn will affect future Factory.ToRESTConfig() calls.
			configFlags.WrapConfigFn = func(cfg *rest.Config) *rest.Config {
				cfg.QPS = -1
				cfg.Burst = -1
				return cfg
			}
		}
		return nil
	}
}

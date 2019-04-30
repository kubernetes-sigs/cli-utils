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

package dy

import (
	"github.com/google/wire"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/dispatch"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/list"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/output"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/parse"
)

// ProviderSet provides wiring for initializing types.
var ProviderSet = wire.NewSet(wire.Struct(new(output.CommandOutputWriter), "*"),
	wire.Struct(new(list.CommandLister), "*"),
	wire.Struct(new(parse.CommandParser), "*"),
	wire.Struct(new(dispatch.Dispatcher), "*"),
	wire.Struct(new(CommandBuilder), "*"))

// CommandBuilder creates dynamically generated commands from annotations on CRDs.
type CommandBuilder struct {
	// KubernetesClient is used to make requests
	KubernetesClient *kubernetes.Clientset

	// Lister lists Commands from CRDs
	Lister *list.CommandLister

	// Parser parses Commands from CRDs into cobra Commands
	Parser *parse.CommandParser

	// Parser parses Commands from CRDs into cobra Commands
	Dispatcher *dispatch.Dispatcher

	// Writer writes templatized output
	Writer *output.CommandOutputWriter
}

// Build adds dynamic Commands to the root Command.
func (b *CommandBuilder) Build(root *cobra.Command, options *v1.ListOptions) error {
	// list commands from CRDs
	l, err := b.Lister.List(options)
	if err != nil {
		return err
	}

	for i := range l.Items {
		cmd := l.Items[i]

		// parse the data from the CRD into a cobra Command
		parsed := b.build(&cmd)

		// add the cobra Command to the root Command
		parse.AddAtPath(root, parsed, cmd.Command.Path)
	}
	return nil
}

func (b *CommandBuilder) build(cmd *v1alpha1.ResourceCommand) *cobra.Command {
	cobracmd, values := b.Parser.Parse(&cmd.Command)
	cobracmd.RunE = func(c *cobra.Command, args []string) error {
		// Add the namespace flag value.  This is necessary because it is set by the
		// common cli flags rather than from the command itself.
		ns := cobracmd.Flag("namespace").Value.String()
		values.Flags.Strings["namespace"] = &ns

		// Do the requests
		if err := b.Dispatcher.Dispatch(cmd, &values); err != nil {
			return err
		}

		// Write the output
		if values.IsDryRun() != true {
			return b.Writer.Write(cmd, &values)
		}
		return nil
	}
	return cobracmd
}

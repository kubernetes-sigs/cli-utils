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

package output

import (
	"bytes"
	"fmt"
	"io"
	"text/template"

	clidynamic "sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/parse"
)

// CommandOutputWriter writes command Response values
type CommandOutputWriter struct {
	// Output is the output for the command
	Output io.Writer
}

// Write parses the outputTemplate and executes it with values, writing the output to writer.
func (w *CommandOutputWriter) Write(cmd *clidynamic.ResourceCommand, values *parse.Values) error {
	// Do nothing if there is no output template defined
	if cmd.Output == "" {
		return nil
	}

	// Generate the output from the template and flag + response values
	temp, err := template.New(cmd.Command.Use + "-output-template").Parse(cmd.Output)
	if err != nil {
		return err
	}
	buff := &bytes.Buffer{}
	if err := temp.Execute(buff, values); err != nil {
		return err
	}

	// Print the output
	fmt.Fprintf(w.Output, buff.String())
	return nil
}

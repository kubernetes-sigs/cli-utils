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

package parse

import (
	"github.com/spf13/cobra"
	clidynamic "sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
)

// CommandParser parses clidnamic.Commands into cobra.Commands
type CommandParser struct{}

// Parse parses the dynamic ResourceCommand into a cobra ResourceCommand
func (p *CommandParser) Parse(cmd *clidynamic.Command) (*cobra.Command, Values) {
	values := Values{}

	// create the cobra command by copying values from the cli
	cbra := &cobra.Command{
		Use:        cmd.Use,
		Short:      cmd.Short,
		Long:       cmd.Long,
		Example:    cmd.Example,
		Version:    cmd.Version,
		Deprecated: cmd.Deprecated,
		Aliases:    cmd.Aliases,
		SuggestFor: cmd.SuggestFor,
	}

	// Register the cobra flags in the values structure
	for i := range cmd.Flags {
		cmdFlag := cmd.Flags[i]
		switch cmdFlag.Type {
		case clidynamic.String:
			if values.Flags.Strings == nil {
				values.Flags.Strings = map[string]*string{}
			}
			// Create a string flag and register it
			values.Flags.Strings[cmdFlag.Name] = cbra.Flags().String(cmdFlag.Name, cmdFlag.StringValue, cmdFlag.Description)
		case clidynamic.StringSlice:
			if values.Flags.StringSlices == nil {
				values.Flags.StringSlices = map[string]*[]string{}
			}
			// Create a string slice flag and register it
			values.Flags.StringSlices[cmdFlag.Name] = cbra.Flags().StringSlice(
				cmdFlag.Name, cmdFlag.StringSliceValue, cmdFlag.Description)
		case clidynamic.Int:
			if values.Flags.Ints == nil {
				values.Flags.Ints = map[string]*int32{}
			}
			// Create an int flag and register it
			values.Flags.Ints[cmdFlag.Name] = cbra.Flags().Int32(cmdFlag.Name, cmdFlag.IntValue, cmdFlag.Description)
		case clidynamic.Float:
			if values.Flags.Floats == nil {
				values.Flags.Floats = map[string]*float64{}
			}
			// Create a float flag and register it
			values.Flags.Floats[cmdFlag.Name] = cbra.Flags().Float64(cmdFlag.Name, cmdFlag.FloatValue, cmdFlag.Description)
		case clidynamic.Bool:
			if values.Flags.Bools == nil {
				values.Flags.Bools = map[string]*bool{}
			}
			// Create a bool flag and register it
			values.Flags.Bools[cmdFlag.Name] = cbra.Flags().Bool(cmdFlag.Name, cmdFlag.BoolValue, cmdFlag.Description)
		}
		if cmdFlag.Required != nil && *cmdFlag.Required == true {
			cbra.MarkFlagRequired(cmdFlag.Name)
		}
	}

	// Add the dry-run flag
	if values.Flags.Bools == nil {
		values.Flags.Bools = map[string]*bool{}
	}
	dr := cbra.Flags().Bool("dry-run", false,
		"If true, only print the objects that would be sent without sending them.")
	values.Flags.Bools["dry-run"] = dr
	return cbra, values
}

// Values contains input flag values and output response values
type Values struct {
	// Flags are values provided by the user on the command line
	Flags Flags

	// Responses are values provided by the apiserver in a response
	Responses Flags
}

// Flags contains flag values setup for the cobra command
type Flags struct {
	// Strings contains a map of flag names to string values
	Strings map[string]*string

	// Ints contains a map of flag names to int values
	Ints map[string]*int32

	// Bools contains a map of flag names to bool values
	Bools map[string]*bool

	// Floats contains a map of flag names to flat values
	Floats map[string]*float64

	// StringSlices contains a map of flag names to string slice values
	StringSlices map[string]*[]string
}

// IsDryRun returns true if the command is running in dry-run mode
func (v Values) IsDryRun() bool {
	if dr, ok := v.Flags.Bools["dry-run"]; ok {
		return *dr
	}
	return false
}

// AddAtPath adds the subcmd to root at the provided path.  An empty path will add subcmd as a sub-command of root.
func AddAtPath(root, subcmd *cobra.Command, path []string) {
	next := root
	// For each element on the Path
	for i := range path {
		p := path[i]
		// Make sure the subcommand exists
		found := false
		for i := range next.Commands() {
			c := next.Commands()[i]
			if c.Use == p {
				// Found, continue on to next part of the Path
				next = c
				found = true
				break
			}
		}

		if found == false {
			// Missing, create the sub-command
			cbra := &cobra.Command{Use: p}
			next.AddCommand(cbra)
			next = cbra
		}
	}

	next.AddCommand(subcmd)
}

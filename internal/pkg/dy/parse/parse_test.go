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

package parse_test

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/parse"
)

func TestParseDryRunFlag(t *testing.T) {
	instance := parse.CommandParser{}
	cmd := &v1alpha1.Command{}
	cobracmd, values := instance.Parse(cmd)

	assert.Equal(t, false, *values.Flags.Bools["dry-run"])
	// Check the flags get updated
	cobracmd.Flags().Parse([]string{
		"--dry-run",
	})
	assert.Equal(t, true, *values.Flags.Bools["dry-run"])
}

func TestCommandParser_Parse_StringFlags(t *testing.T) {
	instance := parse.CommandParser{}
	cmd := &v1alpha1.Command{
		Flags: []v1alpha1.Flag{
			{
				Name:        "string-flag-1",
				StringValue: "hello world 1",
				Type:        v1alpha1.String,
				Description: "string-flag-description 1",
			},
			{
				Name:        "string-flag-2",
				StringValue: "hello world 2",
				Type:        v1alpha1.String,
				Description: "string-flag-description 2",
			},
			{
				Name: "string-flag-3",
				Type: v1alpha1.String,
			},
		},
	}
	cobracmd, values := instance.Parse(cmd)

	// Check the Descriptions
	assert.Equal(t, "string-flag-description 1", cobracmd.Flag("string-flag-1").Usage)
	assert.Equal(t, "string-flag-description 2", cobracmd.Flag("string-flag-2").Usage)
	assert.Equal(t, "", cobracmd.Flag("string-flag-3").Usage)

	// Check defai;t Values
	assert.Equal(t, "hello world 1", *values.Flags.Strings["string-flag-1"])
	assert.Equal(t, "hello world 2", *values.Flags.Strings["string-flag-2"])
	assert.Equal(t, "", *values.Flags.Strings["string-flag-3"])

	// Check the flags get updated
	cobracmd.Flags().Parse([]string{
		"--string-flag-1=foo 1",
	})
	assert.Equal(t, "foo 1", *values.Flags.Strings["string-flag-1"])
	assert.Equal(t, "hello world 2", *values.Flags.Strings["string-flag-2"])
	assert.Equal(t, "", *values.Flags.Strings["string-flag-3"])
}

func TestCommandParser_Parse_StringSliceFlags(t *testing.T) {
	instance := parse.CommandParser{}
	cmd := &v1alpha1.Command{
		Flags: []v1alpha1.Flag{
			{
				Name:             "string-slice-flag-1",
				StringSliceValue: []string{"hello1", "world1"},
				Type:             v1alpha1.StringSlice,
				Description:      "string-slice-flag-description 1",
			},
			{
				Name:             "string-slice-flag-2",
				StringSliceValue: []string{"hello2", "world2"},
				Type:             v1alpha1.StringSlice,
				Description:      "string-slice-flag-description 2",
			},
			{
				Name: "string-slice-flag-3",
				Type: v1alpha1.StringSlice,
			},
		},
	}
	cobracmd, values := instance.Parse(cmd)

	// Check the Descriptions
	assert.Equal(t, "string-slice-flag-description 1", cobracmd.Flag("string-slice-flag-1").Usage)
	assert.Equal(t, "string-slice-flag-description 2", cobracmd.Flag("string-slice-flag-2").Usage)
	assert.Equal(t, "", cobracmd.Flag("string-slice-flag-3").Usage)

	// Check default Values
	assert.Equal(t, []string{"hello1", "world1"}, *values.Flags.StringSlices["string-slice-flag-1"])
	assert.Equal(t, []string{"hello2", "world2"}, *values.Flags.StringSlices["string-slice-flag-2"])
	assert.Equal(t, []string(nil), *values.Flags.StringSlices["string-slice-flag-3"])

	// Check the flags get updated
	cobracmd.Flags().Parse([]string{
		"--string-slice-flag-1=foo1",
		"--string-slice-flag-1=bar1",
		"--string-slice-flag-1=11",
		"--string-slice-flag-3=foo3",
		"--string-slice-flag-3=baz3",
	})
	assert.Equal(t, []string{"foo1", "bar1", "11"}, *values.Flags.StringSlices["string-slice-flag-1"])
	assert.Equal(t, []string{"hello2", "world2"}, *values.Flags.StringSlices["string-slice-flag-2"])
	assert.Equal(t, []string{"foo3", "baz3"}, *values.Flags.StringSlices["string-slice-flag-3"])
}

func TestCommandParser_Parse_IntFlags(t *testing.T) {
	instance := parse.CommandParser{}
	cmd := &v1alpha1.Command{
		Flags: []v1alpha1.Flag{
			{
				Name:        "int-flag-1",
				IntValue:    1,
				Type:        v1alpha1.Int,
				Description: "int flag 1 description",
			},
			{
				Name:        "int-flag-2",
				IntValue:    2,
				Type:        v1alpha1.Int,
				Description: "int-flag-2-description",
			},
			{
				Name: "int-flag-3",
				Type: v1alpha1.Int,
			},
		},
	}
	cobracmd, values := instance.Parse(cmd)

	// Check the Descriptions
	assert.Equal(t, "int flag 1 description", cobracmd.Flag("int-flag-1").Usage)
	assert.Equal(t, "int-flag-2-description", cobracmd.Flag("int-flag-2").Usage)
	assert.Equal(t, "", cobracmd.Flag("int-flag-3").Usage)

	// Check default Values
	assert.Equal(t, int32(1), *values.Flags.Ints["int-flag-1"])
	assert.Equal(t, int32(2), *values.Flags.Ints["int-flag-2"])
	assert.Equal(t, int32(0), *values.Flags.Ints["int-flag-3"])

	// Check the flags get updated
	cobracmd.Flags().Parse([]string{
		"--int-flag-1=10",
		"--int-flag-3=3",
	})
	assert.Equal(t, int32(10), *values.Flags.Ints["int-flag-1"])
	assert.Equal(t, int32(2), *values.Flags.Ints["int-flag-2"])
	assert.Equal(t, int32(3), *values.Flags.Ints["int-flag-3"])
}

func TestCommandParser_Parse_FloatFlags(t *testing.T) {
	instance := parse.CommandParser{}
	cmd := &v1alpha1.Command{
		Flags: []v1alpha1.Flag{
			{
				Name:        "float-flag-1",
				FloatValue:  1.1,
				Type:        v1alpha1.Float,
				Description: "float flag 1 description",
			},
			{
				Name:        "float-flag-2",
				FloatValue:  2.2,
				Type:        v1alpha1.Float,
				Description: "float-flag-2-description",
			},
			{
				Name: "float-flag-3",
				Type: v1alpha1.Float,
			},
		},
	}
	cobracmd, values := instance.Parse(cmd)

	// Check the Descriptions
	assert.Equal(t, "float flag 1 description", cobracmd.Flag("float-flag-1").Usage)
	assert.Equal(t, "float-flag-2-description", cobracmd.Flag("float-flag-2").Usage)
	assert.Equal(t, "", cobracmd.Flag("float-flag-3").Usage)

	// Check default Values
	assert.Equal(t, 1.1, *values.Flags.Floats["float-flag-1"])
	assert.Equal(t, 2.2, *values.Flags.Floats["float-flag-2"])
	assert.Equal(t, 0.0, *values.Flags.Floats["float-flag-3"])

	// Check the flags get updated
	cobracmd.Flags().Parse([]string{
		"--float-flag-1=10.10",
		"--float-flag-3=3.3",
	})
	assert.Equal(t, 10.10, *values.Flags.Floats["float-flag-1"])
	assert.Equal(t, 2.2, *values.Flags.Floats["float-flag-2"])
	assert.Equal(t, 3.3, *values.Flags.Floats["float-flag-3"])
}

func TestCommandParser_Parse_BoolFlags(t *testing.T) {
	instance := parse.CommandParser{}
	cmd := &v1alpha1.Command{
		Flags: []v1alpha1.Flag{
			{
				Name:        "bool-flag-1",
				BoolValue:   true,
				Type:        v1alpha1.Bool,
				Description: "bool flag 1 description",
			},
			{
				Name:        "bool-flag-2",
				BoolValue:   false,
				Type:        v1alpha1.Bool,
				Description: "bool-flag-2-description",
			},
			{
				Name: "bool-flag-3",
				Type: v1alpha1.Bool,
			},
		},
	}
	cobracmd, values := instance.Parse(cmd)

	// Check the Descriptions
	assert.Equal(t, "bool flag 1 description", cobracmd.Flag("bool-flag-1").Usage)
	assert.Equal(t, "bool-flag-2-description", cobracmd.Flag("bool-flag-2").Usage)
	assert.Equal(t, "", cobracmd.Flag("bool-flag-3").Usage)

	// Check default Values
	assert.Equal(t, true, *values.Flags.Bools["bool-flag-1"])
	assert.Equal(t, false, *values.Flags.Bools["bool-flag-2"])
	assert.Equal(t, false, *values.Flags.Bools["bool-flag-3"])

	// Check the flags get updated
	cobracmd.Flags().Parse([]string{
		"--bool-flag-1=false",
		"--bool-flag-3=true",
	})
	assert.Equal(t, false, *values.Flags.Bools["bool-flag-1"])
	assert.Equal(t, false, *values.Flags.Bools["bool-flag-2"])
	assert.Equal(t, true, *values.Flags.Bools["bool-flag-3"])
}

func TestCommandParser_Parse(t *testing.T) {
	instance := parse.CommandParser{}
	cmd := &v1alpha1.Command{
		Version:    "v1",
		Example:    "foo example",
		Use:        "foo",
		Aliases:    []string{"a1", "a2"},
		Long:       "long description",
		Short:      "short description",
		SuggestFor: []string{"suggest", "for"},
		Deprecated: "deprecated",
	}
	cobracmd, _ := instance.Parse(cmd)

	assert.Equal(t, "v1", cobracmd.Version)
	assert.Equal(t, "foo example", cobracmd.Example)
	assert.Equal(t, "foo", cobracmd.Use)
	assert.Equal(t, []string{"a1", "a2"}, cobracmd.Aliases)
	assert.Equal(t, "long description", cobracmd.Long)
	assert.Equal(t, "short description", cobracmd.Short)
	assert.Equal(t, []string{"suggest", "for"}, cobracmd.SuggestFor)
	assert.Equal(t, "deprecated", cobracmd.Deprecated)
}

func TestCommandParser_Parse_RequiredFlags(t *testing.T) {
	instance := parse.CommandParser{}
	required := true
	notrequired := false
	cmd := &v1alpha1.Command{
		Flags: []v1alpha1.Flag{
			{
				Name:        "float-flag-1",
				Type:        v1alpha1.Float,
				Description: "float flag 1 description",
				Required:    &required,
			},
			{
				Name:        "float-flag-2",
				FloatValue:  2.2,
				Type:        v1alpha1.Float,
				Description: "float-flag-2-description",
				Required:    &notrequired,
			},
			{
				Name: "float-flag-3",
				Type: v1alpha1.Float,
			},
		},
	}
	cobracmd, _ := instance.Parse(cmd)
	cobracmd.Run = func(cmd *cobra.Command, args []string) {}
	cobracmd.Flags().Parse([]string{})
	err := cobracmd.Execute()
	assert.Error(t, err)

	cobracmd, _ = instance.Parse(cmd)
	cobracmd.Run = func(cmd *cobra.Command, args []string) {}
	cobracmd.Flags().Parse([]string{"--float-flag-1=10.10"})
	err = cobracmd.Execute()
	assert.NoError(t, err)
}

func TestAddAtPath(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	child1 := &cobra.Command{Use: "child1"}
	child2 := &cobra.Command{Use: "child2"}
	parse.AddAtPath(root, child1, []string{"path", "to"})
	parse.AddAtPath(root, child2, []string{"path", "to"})

	assert.Equal(t, 1, len(root.Commands()))
	assert.Equal(t, "path", root.Commands()[0].Use)
	assert.Equal(t, 1, len(root.Commands()[0].Commands()))
	assert.Equal(t, "to", root.Commands()[0].Commands()[0].Use)
	assert.Equal(t, 2, len(root.Commands()[0].Commands()[0].Commands()))

	assert.Equal(t, "root path to child1", child1.CommandPath())
	assert.Equal(t, "root path to child2", child2.CommandPath())
}

func TestValues_IsDryRun(t *testing.T) {
	v := &parse.Values{}
	assert.Equal(t, false, v.IsDryRun())

	v.Flags.Bools = map[string]*bool{}
	assert.Equal(t, false, v.IsDryRun())

	dr := true
	v.Flags.Bools["dry-run"] = &dr
	assert.Equal(t, true, v.IsDryRun())

	dr = false
	assert.Equal(t, false, v.IsDryRun())
}

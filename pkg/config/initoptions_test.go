// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var ioStreams = genericclioptions.IOStreams{}

func TestComplete(t *testing.T) {
	tests := map[string]struct {
		args    []string
		isError bool
	}{
		"Empty args returns error": {
			args:    []string{},
			isError: true,
		},
		"More than one argument should fail": {
			args:    []string{"foo", "bar"},
			isError: true,
		},
		"Non-directory arg should fail": {
			args:    []string{"foo"},
			isError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			io := NewInitOptions(ioStreams)
			err := io.Complete(tc.args)
			if tc.isError && err == nil {
				t.Errorf("Expected error, but did not receive one")
			}
		})
	}
}

func TestDefaultGroupName(t *testing.T) {
	tests := map[string]struct {
		seed     int64
		dir      string
		expected string
	}{
		"Basic Seed/Dir": {
			seed:     31,
			dir:      "bar",
			expected: "bar-851636",
		},
		"Hierarchical directory": {
			seed:     31,
			dir:      "foo/bar",
			expected: "bar-851636",
		},
		"Absolute directory": {
			seed:     31,
			dir:      "/tmp/foo/bar",
			expected: "bar-851636",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			io := NewInitOptions(ioStreams)
			io.Seed = tc.seed
			io.Dir = tc.dir
			actual := io.defaultGroupName()
			if tc.expected != actual {
				t.Errorf("Expected group name (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestValidateGroupName(t *testing.T) {
	tests := map[string]struct {
		groupName string
		isValid   bool
	}{
		"Empty Groupname fails": {
			groupName: "",
			isValid:   false,
		},
		"Groupname greater than sixty-three chars fails": {
			groupName: "88888888888888888888888888888888888888888888888888888888888888888",
			isValid:   false,
		},
		"Non-allowed characters fails": {
			groupName: "&foo",
			isValid:   false,
		},
		"Initial dot fails": {
			groupName: ".foo",
			isValid:   false,
		},
		"Initial dash fails": {
			groupName: "-foo",
			isValid:   false,
		},
		"Initial underscore fails": {
			groupName: "_foo",
			isValid:   false,
		},
		"Trailing dot fails": {
			groupName: "foo.",
			isValid:   false,
		},
		"Trailing dash fails": {
			groupName: "foo-",
			isValid:   false,
		},
		"Trailing underscore fails": {
			groupName: "foo_",
			isValid:   false,
		},
		"Initial digit succeeds": {
			groupName: "90-foo.bar_test",
			isValid:   true,
		},
		"Allowed characters succeed": {
			groupName: "f_oo90bar-t.est90",
			isValid:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualValid := validateGroupName(tc.groupName)
			if tc.isValid != actualValid {
				t.Errorf("Groupname: %s. Expected valid (%t), got (%t)", tc.groupName, tc.isValid, actualValid)
			}
		})
	}
}

func TestFillInValues(t *testing.T) {
	tests := map[string]struct {
		namespace string
		groupname string
	}{
		"Basic namespace/groupname": {
			namespace: "foo",
			groupname: "bar",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			io := NewInitOptions(ioStreams)
			io.Namespace = tc.namespace
			io.GroupName = tc.groupname
			actual := io.fillInValues()
			expectedLabel := fmt.Sprintf("cli-utils.sigs.k8s.io/inventory-id: %s", tc.groupname)
			if !strings.Contains(actual, expectedLabel) {
				t.Errorf("\nExpected label (%s) not found in grouping object: %s\n", expectedLabel, actual)
			}
			expectedNamespace := fmt.Sprintf("namespace: %s", tc.namespace)
			if !strings.Contains(actual, expectedNamespace) {
				t.Errorf("\nExpected namespace (%s) not found in grouping object: %s\n", expectedNamespace, actual)
			}
			if !strings.Contains(actual, "kind: ConfigMap") {
				t.Errorf("\nExpected `kind: ConfigMap` not found in grouping object: %s\n", actual)
			}
		})
	}
}

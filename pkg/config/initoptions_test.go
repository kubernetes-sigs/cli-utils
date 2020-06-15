// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var ioStreams = genericclioptions.IOStreams{}

// writeFile writes a file under the test directory
func writeFile(t *testing.T, path string, value []byte) {
	err := ioutil.WriteFile(path, value, 0600)
	if !assert.NoError(t, err) {
		assert.FailNow(t, err.Error())
	}
}

var readFileA = []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: objA
  namespace: namespaceA
`)

var readFileB = []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: objB
  namespace: namespaceB
`)

var readFileC = []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: objC
`)

func TestComplete(t *testing.T) {
	d1, err := ioutil.TempDir("", "test-dir")
	if !assert.NoError(t, err) {
		assert.FailNow(t, err.Error())
	}
	defer os.RemoveAll(d1)
	d2, err := ioutil.TempDir("", "test-dir")
	if !assert.NoError(t, err) {
		assert.FailNow(t, err.Error())
	}
	defer os.RemoveAll(d2)

	writeFile(t, filepath.Join(d1, "a_test.yaml"), readFileA)
	writeFile(t, filepath.Join(d1, "b_test.yaml"), readFileB)
	writeFile(t, filepath.Join(d2, "b_test.yaml"), readFileC)

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
		"More than one namespace should fail": {
			args:    []string{d1},
			isError: true,
		},
		"No namespace set is fine": {
			args:    []string{d2},
			isError: false,
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

func TestDefaultInventoryID(t *testing.T) {
	io := NewInitOptions(ioStreams)
	actual, err := io.defaultInventoryID()
	if err != nil {
		t.Errorf("Unxpected error during UUID generation: %v", err)
	}
	// Example UUID: dd647113-a354-48fa-9b93-cc1b7a85aadb
	var uuidRegexp = `^[a-z0-9]{8}\-[a-z0-9]{4}\-[a-z0-9]{4}\-[a-z0-9]{4}\-[a-z0-9]{12}$`
	re := regexp.MustCompile(uuidRegexp)
	if !re.MatchString(actual) {
		t.Errorf("Expected UUID; got (%s)", actual)
	}
}

func TestValidateInventoryID(t *testing.T) {
	tests := map[string]struct {
		inventoryID string
		isValid     bool
	}{
		"Empty InventoryID fails": {
			inventoryID: "",
			isValid:     false,
		},
		"InventoryID greater than sixty-three chars fails": {
			inventoryID: "88888888888888888888888888888888888888888888888888888888888888888",
			isValid:     false,
		},
		"Non-allowed characters fails": {
			inventoryID: "&foo",
			isValid:     false,
		},
		"Initial dot fails": {
			inventoryID: ".foo",
			isValid:     false,
		},
		"Initial dash fails": {
			inventoryID: "-foo",
			isValid:     false,
		},
		"Initial underscore fails": {
			inventoryID: "_foo",
			isValid:     false,
		},
		"Trailing dot fails": {
			inventoryID: "foo.",
			isValid:     false,
		},
		"Trailing dash fails": {
			inventoryID: "foo-",
			isValid:     false,
		},
		"Trailing underscore fails": {
			inventoryID: "foo_",
			isValid:     false,
		},
		"Initial digit succeeds": {
			inventoryID: "90-foo.bar_test",
			isValid:     true,
		},
		"Allowed characters succeed": {
			inventoryID: "f_oo90bar-t.est90",
			isValid:     true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualValid := validateInventoryID(tc.inventoryID)
			if tc.isValid != actualValid {
				t.Errorf("InventoryID: %s. Expected valid (%t), got (%t)", tc.inventoryID, tc.isValid, actualValid)
			}
		})
	}
}

func TestFillInValues(t *testing.T) {
	tests := map[string]struct {
		namespace   string
		inventoryID string
	}{
		"Basic namespace/inventoryID": {
			namespace:   "foo",
			inventoryID: "bar",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			io := NewInitOptions(ioStreams)
			io.Namespace = tc.namespace
			io.InventoryID = tc.inventoryID
			actual := io.fillInValues()
			expectedLabel := fmt.Sprintf("cli-utils.sigs.k8s.io/inventory-id: %s", tc.inventoryID)
			if !strings.Contains(actual, expectedLabel) {
				t.Errorf("\nExpected label (%s) not found in inventory object: %s\n", expectedLabel, actual)
			}
			expectedNamespace := fmt.Sprintf("namespace: %s", tc.namespace)
			if !strings.Contains(actual, expectedNamespace) {
				t.Errorf("\nExpected namespace (%s) not found in inventory object: %s\n", expectedNamespace, actual)
			}
			if !strings.Contains(actual, "kind: ConfigMap") {
				t.Errorf("\nExpected `kind: ConfigMap` not found in inventory object: %s\n", actual)
			}
		})
	}
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

// writeFile writes a file under the test directory
func writeFile(t *testing.T, path string, value []byte) {
	err := os.WriteFile(path, value, 0600)
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

var readFileD = []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: objD
  namespace: namespaceD
  annotations:
    config.kubernetes.io/local-config: "true"
`)

var readFileE = []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: objE
  namespace: namespaceA
`)

var readFileF = []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: namespaceA
`)

func TestComplete(t *testing.T) {
	tests := map[string]struct {
		args               []string
		files              map[string][]byte
		isError            bool
		expectedErrMessage string
		expectedNamespace  string
	}{
		"Empty args returns error": {
			args:               []string{},
			isError:            true,
			expectedErrMessage: "need one 'directory' arg; have 0",
		},
		"More than one argument should fail": {
			args:               []string{"foo", "bar"},
			isError:            true,
			expectedErrMessage: "need one 'directory' arg; have 2",
		},
		"Non-directory arg should fail": {
			args:               []string{"foo"},
			isError:            true,
			expectedErrMessage: "invalid directory argument: foo",
		},
		"More than one namespace should fail": {
			args: []string{},
			files: map[string][]byte{
				"a_test.yaml": readFileA,
				"b_test.yaml": readFileB,
			},
			isError:            true,
			expectedErrMessage: "resources belong to different namespaces",
		},
		"If at least one resource doesn't have namespace, it should use the default": {
			args: []string{},
			files: map[string][]byte{
				"b_test.yaml": readFileB,
				"c_test.yaml": readFileC,
			},
			isError:           false,
			expectedNamespace: "foo",
		},
		"No resources without namespace should use the default namespace": {
			args: []string{},
			files: map[string][]byte{
				"c_test.yaml": readFileC,
			},
			isError:           false,
			expectedNamespace: "foo",
		},
		"Resources with the LocalConfig annotation should be ignored": {
			args: []string{},
			files: map[string][]byte{
				"b_test.yaml": readFileB,
				"d_test.yaml": readFileD,
			},
			isError:           false,
			expectedNamespace: "foo",
		},
		"If all resources have the LocalConfig annotation use the default namespace": {
			args: []string{},
			files: map[string][]byte{
				"d_test.yaml": readFileD,
			},
			isError:           false,
			expectedNamespace: "foo",
		},
		"Cluster-scoped resources are ignored in namespace calculation": {
			args: []string{},
			files: map[string][]byte{
				"a_test.yaml": readFileA,
				"e_test.yaml": readFileE,
				"f_test.yaml": readFileF,
			},
			isError:           false,
			expectedNamespace: "foo",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var err error
			dir, err := os.MkdirTemp("", "test-dir")
			if !assert.NoError(t, err) {
				assert.FailNow(t, err.Error())
			}
			defer os.RemoveAll(dir)

			for fileName, fileContent := range tc.files {
				writeFile(t, filepath.Join(dir, fileName), fileContent)
			}
			if len(tc.files) > 0 {
				tc.args = append(tc.args, dir)
			}

			tf := cmdtesting.NewTestFactory().WithNamespace("foo")
			defer tf.Cleanup()
			ioStreams, _, out, _ := genericiooptions.NewTestIOStreams()
			io := NewInitOptions(tf, ioStreams)
			err = io.Complete(tc.args)

			if err != nil {
				if !tc.isError {
					t.Errorf("Expected error, but did not receive one")
					return
				}
				assert.Contains(t, err.Error(), tc.expectedErrMessage)
				return
			}
			assert.Contains(t, out.String(), tc.expectedNamespace)
		})
	}
}

func TestFindNamespace(t *testing.T) {
	testCases := map[string]struct {
		namespace         string
		enforceNamespace  bool
		files             map[string][]byte
		expectedNamespace string
	}{
		"fallback to default": {
			namespace:        "foo",
			enforceNamespace: false,
			files: map[string][]byte{
				"a_test.yaml": readFileA,
				"b_test.yaml": readFileB,
			},
			expectedNamespace: "foo",
		},
		"enforce namespace": {
			namespace:        "bar",
			enforceNamespace: true,
			files: map[string][]byte{
				"a_test.yaml": readFileA,
			},
			expectedNamespace: "bar",
		},
		"use namespace from resource if all the same": {
			namespace:        "bar",
			enforceNamespace: false,
			files: map[string][]byte{
				"a_test.yaml": readFileA,
			},
			expectedNamespace: "namespaceA",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			var err error
			dir, err := os.MkdirTemp("", "test-dir")
			if !assert.NoError(t, err) {
				assert.FailNow(t, err.Error())
			}
			defer os.RemoveAll(dir)

			for fileName, fileContent := range tc.files {
				writeFile(t, filepath.Join(dir, fileName), fileContent)
			}

			fakeLoader := &fakeNamespaceLoader{
				namespace:        tc.namespace,
				enforceNamespace: tc.enforceNamespace,
			}

			namespace, err := FindNamespace(fakeLoader, dir)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedNamespace, namespace)
		})
	}
}

type fakeNamespaceLoader struct {
	namespace        string
	enforceNamespace bool
}

func (f *fakeNamespaceLoader) Namespace() (string, bool, error) {
	return f.namespace, f.enforceNamespace, nil
}

func TestDefaultInventoryID(t *testing.T) {
	tf := cmdtesting.NewTestFactory().WithNamespace("foo")
	defer tf.Cleanup()
	ioStreams, _, _, _ := genericiooptions.NewTestIOStreams() // nolint:dogsled
	io := NewInitOptions(tf, ioStreams)
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
			tf := cmdtesting.NewTestFactory().WithNamespace("foo")
			defer tf.Cleanup()
			ioStreams, _, _, _ := genericiooptions.NewTestIOStreams()
			io := NewInitOptions(tf, ioStreams)
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
			matched, err := regexp.MatchString(`name: inventory-\d{8}\n`, actual)
			if err != nil {
				t.Errorf("unexpected error parsing inventory name: %s", err)
			}
			if !matched {
				t.Errorf("expected inventory name (e.g. inventory-12345678), got (%s)", actual)
			}
			if !strings.Contains(actual, "kind: ConfigMap") {
				t.Errorf("\nExpected `kind: ConfigMap` not found in inventory object: %s\n", actual)
			}
		})
	}
}

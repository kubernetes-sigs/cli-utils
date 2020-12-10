// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golangplus/testing/assert"
)

func TestCheckRequiredSettersSet(t *testing.T) {
	var tests = []struct {
		name             string
		inputOpenAPIfile string
		expectedError    bool
	}{
		{
			name: "required true, isSet false",
			inputOpenAPIfile: `
apiVersion: v1alpha1
kind: OpenAPIfile
openAPI:
  definitions:
    io.k8s.cli.setters.gcloud.project.projectNumber:
      description: hello world
      x-k8s-cli:
        setter:
          name: gcloud.project.projectNumber
          value: "123"
          setBy: me
    io.k8s.cli.setters.replicas:
      description: hello world
      x-k8s-cli:
        setter:
          name: replicas
          value: "3"
          setBy: me
          required: true
          isSet: false
 `,
			expectedError: true,
		},
		{
			name:             "no file, no error",
			inputOpenAPIfile: ``,
			expectedError:    false,
		},
	}
	for i := range tests {
		test := tests[i]
		t.Run(test.name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "")
			assert.NoError(t, err)
			defer os.RemoveAll(dir)
			if test.inputOpenAPIfile != "" {
				err = ioutil.WriteFile(filepath.Join(dir, "Krmfile"), []byte(test.inputOpenAPIfile), 0600)
				if !assert.NoError(t, err) {
					t.FailNow()
				}
			}
			err = CheckForRequiredSetters(dir)
			if test.expectedError && !assert.Error(t, err) {
				t.FailNow()
			}
			if !test.expectedError && !assert.NoError(t, err) {
				t.FailNow()
			}
		})
	}
}

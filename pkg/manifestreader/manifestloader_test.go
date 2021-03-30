// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

func TestMReader_Read(t *testing.T) {
	testCases := map[string]struct {
		manifests        map[string]string
		namespace        string
		enforceNamespace bool
		validate         bool
		args             []string

		infosCount int
		namespaces []string
	}{
		"path mReader: namespace should be set if not already present": {
			namespace:        "foo",
			enforceNamespace: true,
			args:             []string{"${reader-test-dir}"},
			infosCount:       1,
			namespaces:       []string{"foo"},
		},
		"stream mReader: namespace should be set if not already present": {
			namespace:        "foo",
			enforceNamespace: true,
			args:             []string{"-"},
			infosCount:       1,
			namespaces:       []string{"foo"},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("test-ns")
			defer tf.Cleanup()

			mapper, err := tf.ToRESTMapper()
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			dir, err := ioutil.TempDir("", "reader-test")
			assert.NoError(t, err)
			p := filepath.Join(dir, "dep.yaml")
			err = ioutil.WriteFile(p, []byte(depManifest), 0600)
			assert.NoError(t, err)
			stringReader := strings.NewReader(depManifest)

			if tc.args[0] == "${reader-test-dir}" {
				tc.args = []string{dir}
			}

			objs, err := mReader(tc.args, stringReader, ReaderOptions{
				Mapper:           mapper,
				Namespace:        tc.namespace,
				EnforceNamespace: tc.enforceNamespace,
				Validate:         tc.validate,
			}).Read()

			assert.NoError(t, err)
			assert.Equal(t, len(objs), tc.infosCount)

			for i, obj := range objs {
				assert.Equal(t, tc.namespaces[i], obj.GetNamespace())
			}
		})
	}
}

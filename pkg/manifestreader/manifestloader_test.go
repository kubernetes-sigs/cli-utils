// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"os"
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
		path             string

		infosCount int
		namespaces []string
	}{
		"path mReader: namespace should be set if not already present": {
			namespace:        "foo",
			enforceNamespace: true,
			path:             "${reader-test-dir}",
			infosCount:       1,
			namespaces:       []string{"foo"},
		},
		"stream mReader: namespace should be set if not already present": {
			namespace:        "foo",
			enforceNamespace: true,
			path:             "-",
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

			dir, err := os.MkdirTemp("", "reader-test")
			assert.NoError(t, err)
			p := filepath.Join(dir, "dep.yaml")
			err = os.WriteFile(p, []byte(depManifest), 0600)
			assert.NoError(t, err)
			stringReader := strings.NewReader(depManifest)

			if tc.path == "${reader-test-dir}" {
				tc.path = dir
			}

			objs, err := mReader(tc.path, stringReader, ReaderOptions{
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

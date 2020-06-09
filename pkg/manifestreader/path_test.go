// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

func TestPathManifestReader_Read(t *testing.T) {
	testCases := map[string]struct {
		manifests        map[string]string
		namespace        string
		enforceNamespace bool
		validate         bool

		infosCount int
		namespaces []string
	}{
		"namespace should be set if not already present": {
			manifests: map[string]string{
				"dep.yaml": depManifest,
			},
			namespace:        "foo",
			enforceNamespace: true,

			infosCount: 1,
			namespaces: []string{"foo"},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("test-ns")
			defer tf.Cleanup()

			dir, err := ioutil.TempDir("", "path-reader-test")
			assert.NoError(t, err)
			for filename, content := range tc.manifests {
				p := filepath.Join(dir, filename)
				err := ioutil.WriteFile(p, []byte(content), 0600)
				assert.NoError(t, err)
			}

			infos, err := (&PathManifestReader{
				Path: dir,
				ReaderOptions: ReaderOptions{
					Factory:          tf,
					Namespace:        tc.namespace,
					EnforceNamespace: tc.enforceNamespace,
					Validate:         tc.validate,
				},
			}).Read()

			assert.NoError(t, err)
			assert.Equal(t, len(infos), tc.infosCount)

			for i, info := range infos {
				assert.Equal(t, tc.namespaces[i], info.Namespace)
			}
		})
	}
}

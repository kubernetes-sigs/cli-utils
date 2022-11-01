// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
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
		"multiple manifests": {
			manifests: map[string]string{
				"dep.yaml": depManifest,
				"cm.yaml":  cmManifest,
			},
			namespace:        "default",
			enforceNamespace: true,

			infosCount: 2,
			namespaces: []string{"default", "default"},
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

			dir, err := os.MkdirTemp("", "path-reader-test")
			assert.NoError(t, err)
			for filename, content := range tc.manifests {
				p := filepath.Join(dir, filename)
				err := os.WriteFile(p, []byte(content), 0600)
				assert.NoError(t, err)
			}

			objs, err := (&PathManifestReader{
				Path: dir,
				ReaderOptions: ReaderOptions{
					Mapper:           mapper,
					Namespace:        tc.namespace,
					EnforceNamespace: tc.enforceNamespace,
					Validate:         tc.validate,
				},
			}).Read()

			assert.NoError(t, err)
			assert.Equal(t, len(objs), tc.infosCount)

			for i, obj := range objs {
				assert.Equal(t, tc.namespaces[i], obj.GetNamespace())
				_, ok := obj.GetAnnotations()[kioutil.PathAnnotation]
				assert.True(t, ok)
			}
		})
	}
}

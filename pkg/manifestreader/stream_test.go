// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

func TestStreamManifestReader_Read(t *testing.T) {
	testCases := map[string]struct {
		manifests        string
		namespace        string
		enforceNamespace bool
		validate         bool

		infosCount int
		namespaces []string
	}{
		"namespace should be set if not already present": {
			manifests:        depManifest,
			namespace:        "foo",
			enforceNamespace: true,

			infosCount: 1,
			namespaces: []string{"foo"},
		},
		"multiple resources": {
			manifests:        depManifest + "\n---\n" + cmManifest,
			namespace:        "bar",
			enforceNamespace: false,

			infosCount: 2,
			namespaces: []string{"bar", "bar"},
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

			stringReader := strings.NewReader(tc.manifests)

			objs, err := (&StreamManifestReader{
				ReaderName: "testReader",
				Reader:     stringReader,
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
			}
		})
	}
}

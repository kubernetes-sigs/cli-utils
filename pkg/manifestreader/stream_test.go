// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

var (
	depManifest = `
kind: Deployment
apiVersion: apps/v1
metadata:
  name: foo
spec:
  replicas: 1
`
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
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("test-ns")
			defer tf.Cleanup()

			stringReader := strings.NewReader(tc.manifests)

			infos, err := (&StreamManifestReader{
				ReaderName: "testReader",
				Reader:     stringReader,
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

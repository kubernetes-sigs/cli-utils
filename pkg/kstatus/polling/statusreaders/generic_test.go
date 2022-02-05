// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakecr "sigs.k8s.io/cli-utils/pkg/kstatus/polling/clusterreader/fake"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	fakemapper "sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	customGVK = schema.GroupVersionKind{
		Group:   "custom.io",
		Version: "v1beta1",
		Kind:    "Custom",
	}
	name      = "Foo"
	namespace = "default"
)

func TestGenericStatusReader(t *testing.T) {
	testCases := map[string]struct {
		result             *status.Result
		err                error
		expectedIdentifier object.ObjMetadata
		expectedStatus     status.Status
	}{
		"successfully computes status": {
			result: &status.Result{
				Status:  status.InProgressStatus,
				Message: "this is a test",
			},
			expectedIdentifier: object.ObjMetadata{
				GroupKind: customGVK.GroupKind(),
				Name:      name,
				Namespace: namespace,
			},
			expectedStatus: status.InProgressStatus,
		},
		"computing status fails": {
			err: fmt.Errorf("this error is a test"),
			expectedIdentifier: object.ObjMetadata{
				GroupKind: customGVK.GroupKind(),
				Name:      name,
				Namespace: namespace,
			},
			expectedStatus: status.UnknownStatus,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeReader := fakecr.NewNoopClusterReader()
			fakeMapper := fakemapper.NewFakeRESTMapper()
			resourceStatusReader := &genericStatusReader{
				mapper: fakeMapper,
				statusFunc: func(u *unstructured.Unstructured) (*status.Result, error) {
					return tc.result, tc.err
				},
			}

			o := &unstructured.Unstructured{}
			o.SetGroupVersionKind(customGVK)
			o.SetName(name)
			o.SetNamespace(namespace)

			resourceStatus, err := resourceStatusReader.ReadStatusForObject(context.Background(), fakeReader, o)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedIdentifier, resourceStatus.Identifier)
			assert.Equal(t, tc.expectedStatus, resourceStatus.Status)
		})
	}
}

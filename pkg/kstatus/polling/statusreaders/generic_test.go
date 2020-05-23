// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"fmt"
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
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
			fakeReader := testutil.NewNoopClusterReader()
			fakeMapper := fakemapper.NewFakeRESTMapper()
			resourceStatusReader := &genericStatusReader{
				reader: fakeReader,
				mapper: fakeMapper,
				statusFunc: func(u *unstructured.Unstructured) (*status.Result, error) {
					return tc.result, tc.err
				},
			}

			o := &unstructured.Unstructured{}
			o.SetGroupVersionKind(customGVK)
			o.SetName(name)
			o.SetNamespace(namespace)

			resourceStatus := resourceStatusReader.ReadStatusForObject(context.Background(), o)

			assert.Equal(t, tc.expectedIdentifier, resourceStatus.Identifier)
			assert.Equal(t, tc.expectedStatus, resourceStatus.Status)
		})
	}
}

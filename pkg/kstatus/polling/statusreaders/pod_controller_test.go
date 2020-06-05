// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"fmt"
	"testing"

	"gotest.tools/assert"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	fakemapper "sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestPodControllerStatusReader(t *testing.T) {
	rsGVK = v1.SchemeGroupVersion.WithKind("ReplicaSet")
	name := "Foo"
	namespace := "Bar"

	testCases := map[string]struct {
		computeStatusResult *status.Result
		computeStatusErr    error
		genResourceStatuses event.ResourceStatuses
		expectedIdentifier  object.ObjMetadata
		expectedStatus      status.Status
	}{
		"successfully computes status": {
			computeStatusResult: &status.Result{
				Status:  status.InProgressStatus,
				Message: "this is a test",
			},
			expectedIdentifier: object.ObjMetadata{
				GroupKind: rsGVK.GroupKind(),
				Name:      name,
				Namespace: namespace,
			},
			expectedStatus: status.InProgressStatus,
		},
		"computing status fails": {
			computeStatusErr: fmt.Errorf("this error is a test"),
			expectedIdentifier: object.ObjMetadata{
				GroupKind: rsGVK.GroupKind(),
				Name:      name,
				Namespace: namespace,
			},
			expectedStatus: status.UnknownStatus,
		},
		"one of the pods has failed status": {
			computeStatusResult: &status.Result{
				Status:  status.InProgressStatus,
				Message: "this is a test",
			},
			genResourceStatuses: event.ResourceStatuses{
				{
					Status: status.CurrentStatus,
				},
				{
					Status: status.InProgressStatus,
				},
				{
					Status: status.FailedStatus,
				},
			},
			expectedIdentifier: object.ObjMetadata{
				GroupKind: rsGVK.GroupKind(),
				Name:      name,
				Namespace: namespace,
			},
			expectedStatus: status.FailedStatus,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeReader := testutil.NewNoopClusterReader()
			fakeMapper := fakemapper.NewFakeRESTMapper()
			podControllerStatusReader := &podControllerStatusReader{
				reader: fakeReader,
				mapper: fakeMapper,
				statusFunc: func(u *unstructured.Unstructured) (*status.Result, error) {
					return tc.computeStatusResult, tc.computeStatusErr
				},
				statusForGenResourcesFunc: fakeStatusForGenResourcesFunc(tc.genResourceStatuses, nil),
			}

			rs := &unstructured.Unstructured{}
			rs.SetGroupVersionKind(rsGVK)
			rs.SetName(name)
			rs.SetNamespace(namespace)

			resourceStatus := podControllerStatusReader.readStatus(context.Background(), rs)

			assert.Equal(t, tc.expectedIdentifier, resourceStatus.Identifier)
			assert.Equal(t, tc.expectedStatus, resourceStatus.Status)
		})
	}
}

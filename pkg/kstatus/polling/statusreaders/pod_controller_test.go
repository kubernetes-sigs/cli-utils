// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakecr "sigs.k8s.io/cli-utils/pkg/kstatus/polling/clusterreader/fake"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
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
			fakeReader := fakecr.NewNoopClusterReader()
			fakeMapper := fakemapper.NewFakeRESTMapper()
			podControllerStatusReader := &podControllerStatusReader{
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

			resourceStatus, err := podControllerStatusReader.readStatus(context.Background(), fakeReader, rs)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedIdentifier, resourceStatus.Identifier)
			assert.Equal(t, tc.expectedStatus, resourceStatus.Status)
		})
	}
}

func fakeStatusForGenResourcesFunc(resourceStatuses event.ResourceStatuses, err error) statusForGenResourcesFunc {
	return func(_ context.Context, _ meta.RESTMapper, _ engine.ClusterReader, _ resourceTypeStatusReader,
		_ *unstructured.Unstructured, _ schema.GroupKind, _ ...string) (event.ResourceStatuses, error) {
		return resourceStatuses, err
	}
}

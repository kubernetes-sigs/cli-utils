// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	fakecr "sigs.k8s.io/cli-utils/pkg/kstatus/polling/clusterreader/fake"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/statusreaders/fake"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	fakemapper "sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	currentDeployment = strings.TrimSpace(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
  generation: 1
  namespace: qual
spec:
  selector:
    matchLabels:
      app: app
status:
  observedGeneration: 1
  updatedReplicas: 1
  readyReplicas: 1
  availableReplicas: 1
  replicas: 1
  conditions:
  - type: Progressing 
    status: "True"
    reason: NewReplicaSetAvailable
  - type: Available 
    status: "True"
`)
)

func TestReadStatus(t *testing.T) {
	testCases := map[string]struct {
		identifier             object.ObjMetadata
		readerResource         *unstructured.Unstructured
		readerErr              error
		expectedErr            error
		expectedResourceStatus *event.ResourceStatus
	}{
		"Current resource": {
			identifier:     object.UnstructuredToObjMetadata(testutil.YamlToUnstructured(t, currentDeployment)),
			readerResource: testutil.YamlToUnstructured(t, currentDeployment),
			expectedResourceStatus: &event.ResourceStatus{
				Identifier: object.UnstructuredToObjMetadata(testutil.YamlToUnstructured(t, currentDeployment)),
				Status:     status.CurrentStatus,
				Resource:   testutil.YamlToUnstructured(t, currentDeployment),
				Message:    "Deployment is available. Replicas: 1",
			},
		},
		"Resource not found": {
			identifier: object.UnstructuredToObjMetadata(testutil.YamlToUnstructured(t, currentDeployment)),
			readerErr:  errors.NewNotFound(deploymentGVR.GroupResource(), "test"),
			expectedResourceStatus: &event.ResourceStatus{
				Identifier: object.UnstructuredToObjMetadata(testutil.YamlToUnstructured(t, currentDeployment)),
				Status:     status.NotFoundStatus,
				Message:    "Resource not found",
			},
		},
		"Context cancelled": {
			identifier:  object.UnstructuredToObjMetadata(testutil.YamlToUnstructured(t, currentDeployment)),
			readerErr:   context.Canceled,
			expectedErr: context.Canceled,
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			fakeReader := &fakecr.ClusterReader{
				GetResource: tc.readerResource,
				GetErr:      tc.readerErr,
			}
			fakeMapper := fakemapper.NewFakeRESTMapper(deploymentGVK, replicaSetGVK)

			fakeStatusReader := &fake.StatusReader{}
			statusReader := NewDeploymentResourceReader(fakeMapper, fakeStatusReader)

			rs, err := statusReader.ReadStatus(context.Background(), fakeReader, tc.identifier)

			if tc.expectedErr != nil {
				if err == nil {
					t.Errorf("expected error, but didn't get one")
				} else {
					assert.EqualError(t, err, tc.expectedErr.Error())
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedResourceStatus, rs)
		})
	}
}

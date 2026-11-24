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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestExplainPendingTermination(t *testing.T) {
	deployment := &unstructured.Unstructured{}
	deployment.SetUID("deployment-uid")
	deployment.SetAnnotations(map[string]string{deploymentRevisionAnnotation: "2"})
	replicaSet := func(name, revision string, s status.Status, message string) *event.ResourceStatus {
		resource := &unstructured.Unstructured{}
		resource.SetAnnotations(map[string]string{deploymentRevisionAnnotation: revision})
		resource.SetOwnerReferences([]metav1.OwnerReference{{UID: deployment.GetUID()}})
		return &event.ResourceStatus{Identifier: object.ObjMetadata{Name: name}, Resource: resource, Status: s, Message: message}
	}
	testCases := map[string]struct {
		reason          string
		message         string
		deployment      *unstructured.Unstructured
		replicaSets     []*event.ResourceStatus
		expectedMessage string
	}{
		"adds the status of the new pods": {
			message: "Pending termination: 1",
			replicaSets: []*event.ResourceStatus{
				replicaSet("app-a-old", "1", status.InProgressStatus, "Available: 0/1"),
				replicaSet("app-b-new", "2", status.InProgressStatus, "Ready: 3/4"),
			},
			expectedMessage: "Pending termination: 1. New pods app-b-new-*: Ready: 3/4",
		},
		"adds failed new pods": {
			message: "Pending termination: 1",
			replicaSets: []*event.ResourceStatus{
				replicaSet("app-b-new", "2", status.FailedStatus, "1 pods have failed"),
			},
			expectedMessage: "Pending termination: 1. New pods app-b-new-*: 1 pods have failed",
		},
		"keeps the message when the new pods are ready": {
			message: "Pending termination: 1",
			replicaSets: []*event.ResourceStatus{
				replicaSet("app-a-old", "1", status.InProgressStatus, "Available: 0/1"),
				replicaSet("app-b-new", "2", status.CurrentStatus, "ReplicaSet is available. Replicas: 4"),
			},
			expectedMessage: "Pending termination: 1",
		},
		"keeps the message without a deployment revision": {
			message:    "Pending termination: 1",
			deployment: &unstructured.Unstructured{},
			replicaSets: []*event.ResourceStatus{
				replicaSet("app-b-new", "", status.InProgressStatus, "Ready: 3/4"),
			},
			expectedMessage: "Pending termination: 1",
		},
		"keeps other messages": {
			reason:  "LessUpdated",
			message: "Updated: 3/4",
			replicaSets: []*event.ResourceStatus{
				replicaSet("app-b-new", "2", status.InProgressStatus, "Ready: 2/3"),
			},
			expectedMessage: "Updated: 3/4",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			reason, d := tc.reason, tc.deployment
			if reason == "" {
				reason = "ExtraPods"
			}
			if d == nil {
				d = deployment
			}
			res := &status.Result{
				Status:     status.InProgressStatus,
				Message:    tc.message,
				Conditions: []status.Condition{{Type: status.ConditionReconciling, Status: "True", Reason: reason, Message: tc.message}},
			}
			assert.Equal(t, tc.expectedMessage, explainPendingTermination(res, d, tc.replicaSets))
		})
	}
}

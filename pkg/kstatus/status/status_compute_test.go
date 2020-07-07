// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

var currentTime time.Time = time.Now().UTC().Truncate(time.Second)
var currentTimeStr string = currentTime.Format(time.RFC3339)

func y2u(t *testing.T, spec string) *unstructured.Unstructured {
	j, err := yaml.YAMLToJSON([]byte(spec))
	assert.NoError(t, err)
	u, _, err := unstructured.UnstructuredJSONScheme.Decode(j, nil, nil)
	assert.NoError(t, err)
	return u.(*unstructured.Unstructured)
}

type testSpec struct {
	spec                    string
	expectedStatus          Status
	expectedConditions      []Condition
	absentConditionTypes    []ConditionType
	checkLastTransitionTime bool
}

func runStatusTest(t *testing.T, tc testSpec) {
	res, err := Compute(y2u(t, tc.spec))
	assert.NoError(t, err)
	assert.Equal(t, tc.expectedStatus, res.Status)

	for _, expectedCondition := range tc.expectedConditions {
		found := false
		for _, condition := range res.Conditions {
			if condition.Type != expectedCondition.Type {
				continue
			}
			found = true
			assert.Equal(t, expectedCondition.Status, condition.Status)
			assert.Equal(t, expectedCondition.Reason, condition.Reason)
			if tc.checkLastTransitionTime {
				assert.True(t, expectedCondition.LastTransitionTime.Time.Equal(condition.LastTransitionTime.Time))
			}
		}
		if !found {
			t.Errorf("Expected condition of type %s, but didn't find it", expectedCondition.Type)
		}
	}

	for _, absentConditionType := range tc.absentConditionTypes {
		for _, condition := range res.Conditions {
			if condition.Type == absentConditionType {
				t.Errorf("Expected condition %s to be absent, but found it", absentConditionType)
			}
		}
	}
}

var podNoStatus = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
`

var podReady = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   conditions:
    - type: Ready 
      status: "True"
   phase: Running
`

var podCompletedOK = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Succeeded
   conditions:
    - type: Ready 
      status: "False"
      reason: PodCompleted

`

var podCompletedFail = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Failed
   conditions:
    - type: Ready 
      status: "False"
      reason: PodCompleted
`

var podBeingScheduled = `
apiVersion: v1
kind: Pod
metadata:
   creationTimestamp: %s
   generation: 1
   name: test
   namespace: qual
status:
   phase: Pending
   conditions:
    - type: PodScheduled 
      status: "False"
      reason: Unschedulable
`

var podBeingScheduledWithLastTransitionTime = `
apiVersion: v1
kind: Pod
metadata:
   creationTimestamp: %s
   generation: 1
   name: test
   namespace: qual
status:
   phase: Pending
   conditions:
    - type: PodScheduled 
      status: "False"
      reason: Unschedulable
      lastTransitionTime: %s
`

var podUnschedulable = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Pending
   conditions:
    - type: PodScheduled 
      status: "False"
      reason: Unschedulable
`

var podUnschedulableWithLastTransitionTime = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Pending
   conditions:
    - type: PodScheduled 
      status: "False"
      reason: Unschedulable
      lastTransitionTime: %s
`

var podCrashLooping = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Running
   conditions:
    - type: PodScheduled 
      status: "False"
      reason: Unschedulable
   containerStatuses:
    - name: nginx
      state:
         waiting:
            reason: CrashLoopBackOff
`

// Test coverage using GetConditions
func TestPodStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"podNoStatus": {
			spec:           podNoStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "PodNotObserved",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"podReady": {
			spec:               podReady,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
				ConditionStalled,
			},
		},
		"podCompletedSuccessfully": {
			spec:               podCompletedOK,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
				ConditionStalled,
			},
		},
		"podCompletedFailed": {
			spec:               podCompletedFail,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
				ConditionStalled,
			},
		},
		"podBeingScheduled": {
			spec:           fmt.Sprintf(podBeingScheduled, currentTimeStr),
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionReconciling,
					Status:             corev1.ConditionTrue,
					Reason:             "PodNotScheduled",
					LastTransitionTime: metav1.Time{},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
			checkLastTransitionTime: true,
		},
		"podBeingScheduledWithLastTransitionTime": {
			spec:           fmt.Sprintf(podBeingScheduledWithLastTransitionTime, currentTimeStr, currentTimeStr),
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionReconciling,
					Status:             corev1.ConditionTrue,
					Reason:             "PodNotScheduled",
					LastTransitionTime: metav1.Time{Time: currentTime},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
			checkLastTransitionTime: true,
		},
		"podUnschedulable": {
			spec:           podUnschedulable,
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionStalled,
					Status:             corev1.ConditionTrue,
					Reason:             "PodUnschedulable",
					LastTransitionTime: metav1.Time{},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
		"podUnschedulableWithLastTransitionTime": {
			spec:           fmt.Sprintf(podUnschedulableWithLastTransitionTime, currentTimeStr),
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionStalled,
					Status:             corev1.ConditionTrue,
					Reason:             "PodUnschedulable",
					LastTransitionTime: metav1.Time{Time: currentTime},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
		"podCrashLooping": {
			spec:           podCrashLooping,
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{
				{
					Type:   ConditionStalled,
					Status: corev1.ConditionTrue,
					Reason: "ContainerCrashLooping",
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var pvcNoStatus = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
   generation: 1
   name: test
`
var pvcBound = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Bound
`

func TestPVCStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"pvcNoStatus": {
			spec:           pvcNoStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "NotBound",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"pvcBound": {
			spec:               pvcBound,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var stsNoStatus = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   generation: 1
   name: test
`
var stsBadStatus = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   observedGeneration: 1
   currentReplicas: 1
`

var stsOK = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   generation: 1
   name: test
   namespace: qual
spec:
   replicas: 4
status:
   observedGeneration: 1
   currentReplicas: 4
   readyReplicas: 4
   replicas: 4
`

var stsLessReady = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   generation: 1
   name: test
   namespace: qual
spec:
   replicas: 4
status:
   observedGeneration: 1
   currentReplicas: 4
   readyReplicas: 2
   replicas: 4
`
var stsLessCurrent = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   generation: 1
   name: test
   namespace: qual
spec:
   replicas: 4
status:
   observedGeneration: 1
   currentReplicas: 2
   readyReplicas: 4
   replicas: 4
`
var stsExtraPods = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   generation: 1
   name: test
   namespace: qual
spec:
   replicas: 4
status:
   observedGeneration: 1
   currentReplicas: 4
   readyReplicas: 4
   replicas: 8
`

func TestStsStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"stsNoStatus": {
			spec:           stsNoStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessReplicas",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"stsBadStatus": {
			spec:           stsBadStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessReplicas",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"stsOK": {
			spec:               stsOK,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"stsLessReady": {
			spec:           stsLessReady,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessReady",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"stsLessCurrent": {
			spec:           stsLessCurrent,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessCurrent",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"stsExtraPods": {
			spec:           stsExtraPods,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "ExtraPods",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var dsNoStatus = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   generation: 1
`
var dsBadStatus = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
   generation: 1
status:
   observedGeneration: 1
   currentReplicas: 1
`

var dsOK = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
   generation: 1
status:
   desiredNumberScheduled: 4
   currentNumberScheduled: 4
   updatedNumberScheduled: 4
   numberAvailable: 4
   numberReady: 4
   observedGeneration: 1
`

var dsLessReady = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
   generation: 1
status:
   observedGeneration: 1
   desiredNumberScheduled: 4
   currentNumberScheduled: 4
   updatedNumberScheduled: 4
   numberAvailable: 4
   numberReady: 2
`
var dsLessAvailable = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
   generation: 1
status:
   observedGeneration: 1
   desiredNumberScheduled: 4
   currentNumberScheduled: 4
   updatedNumberScheduled: 4
   numberAvailable: 2
   numberReady: 4
`

func TestDaemonsetStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"dsNoStatus": {
			spec:           dsNoStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "NoDesiredNumber",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"dsBadStatus": {
			spec:           dsBadStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "NoDesiredNumber",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"dsOK": {
			spec:               dsOK,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"dsLessReady": {
			spec:           dsLessReady,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessReady",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"dsLessAvailable": {
			spec:           dsLessAvailable,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessAvailable",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var depNoStatus = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
`

var depOK = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
   namespace: qual
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
`

var depNotProgressing = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
   namespace: qual
spec:
   progressDeadlineSeconds: 45
status:
   observedGeneration: 1
   updatedReplicas: 1
   readyReplicas: 1
   availableReplicas: 1
   replicas: 1
   observedGeneration: 1
   conditions:
    - type: Progressing 
      status: "False"
      reason: Some reason
    - type: Available 
      status: "True"
`

var depNoProgressDeadlineSeconds = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
   namespace: qual
status:
   observedGeneration: 1
   updatedReplicas: 1
   readyReplicas: 1
   availableReplicas: 1
   replicas: 1
   observedGeneration: 1
   conditions:
    - type: Available
      status: "True"
`

var depNotAvailable = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
   namespace: qual
status:
   observedGeneration: 1
   updatedReplicas: 1
   readyReplicas: 1
   availableReplicas: 1
   replicas: 1
   observedGeneration: 1
   conditions:
    - type: Progressing 
      status: "True"
      reason: NewReplicaSetAvailable
    - type: Available 
      status: "False"
`

var depProgressDeadlineExceeded = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
   namespace: qual
status:
   observedGeneration: 1
   updatedReplicas: 1
   readyReplicas: 1
   availableReplicas: 1
   replicas: 1
   observedGeneration: 1
   conditions:
    - type: Progressing 
      status: "True"
      reason: ProgressDeadlineExceeded
    - type: Available 
      status: "False"
`

var depProgressDeadlineExceededWithLastTransitionTime = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   generation: 1
   namespace: qual
status:
   observedGeneration: 1
   updatedReplicas: 1
   readyReplicas: 1
   availableReplicas: 1
   replicas: 1
   observedGeneration: 1
   conditions:
    - type: Progressing 
      status: "True"
      reason: ProgressDeadlineExceeded
      lastTransitionTime: %s
    - type: Available 
      status: "False"
`

func TestDeploymentStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"depNoStatus": {
			spec:           depNoStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessReplicas",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"depOK": {
			spec:               depOK,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"depNotProgressing": {
			spec:           depNotProgressing,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "ReplicaSetNotAvailable",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"depNoProgressDeadlineSeconds": {
			spec:               depNoProgressDeadlineSeconds,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"depNotAvailable": {
			spec:           depNotAvailable,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:               ConditionReconciling,
				Status:             corev1.ConditionTrue,
				Reason:             "DeploymentNotAvailable",
				LastTransitionTime: metav1.Time{},
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"depProgressDeadlineExceeded": {
			spec:           depProgressDeadlineExceeded,
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{{
				Type:               ConditionStalled,
				Status:             corev1.ConditionTrue,
				Reason:             "ProgressDeadlineExceeded",
				LastTransitionTime: metav1.Time{},
			}},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
		"depProgressDeadlineExceededWithLastTransitionTime": {
			spec:           fmt.Sprintf(depProgressDeadlineExceededWithLastTransitionTime, currentTimeStr),
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{{
				Type:               ConditionStalled,
				Status:             corev1.ConditionTrue,
				Reason:             "ProgressDeadlineExceeded",
				LastTransitionTime: metav1.Time{Time: currentTime},
			}},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var rsNoStatus = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   generation: 1
`

var rsOK1 = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   replicas: 2
status:
   observedGeneration: 1
   replicas: 2
   readyReplicas: 2
   availableReplicas: 2
   fullyLabeledReplicas: 2
   conditions:
    - type: ReplicaFailure 
      status: "False"
`

var rsOK2 = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   replicas: 2
status:
   observedGeneration: 1
   fullyLabeledReplicas: 2
   replicas: 2
   readyReplicas: 2
   availableReplicas: 2
`

var rsLessReady = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   replicas: 4
status:
   observedGeneration: 1
   replicas: 4
   readyReplicas: 2
   availableReplicas: 4
   fullyLabeledReplicas: 4
`

var rsLessAvailable = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   replicas: 4
status:
   observedGeneration: 1
   replicas: 4
   readyReplicas: 4
   availableReplicas: 2
   fullyLabeledReplicas: 4
`

var rsReplicaFailure = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   replicas: 4
status:
   observedGeneration: 1
   replicas: 4
   readyReplicas: 4
   fullyLabeledReplicas: 4
   availableReplicas: 4
   conditions:
    - type: ReplicaFailure 
      status: "True"
`

var rsReplicaFailureWithTransitionTime = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   replicas: 4
status:
   observedGeneration: 1
   replicas: 4
   readyReplicas: 4
   fullyLabeledReplicas: 4
   availableReplicas: 4
   conditions:
    - type: ReplicaFailure
      status: "True"
      lastTransitionTime: %s
`

func TestReplicasetStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"rsNoStatus": {
			spec:           rsNoStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessLabelled",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"rsOK1": {
			spec:               rsOK1,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"rsOK2": {
			spec:               rsOK2,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"rsLessAvailable": {
			spec:           rsLessAvailable,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessAvailable",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"rsLessReady": {
			spec:           rsLessReady,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LessReady",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"rsReplicaFailure": {
			spec:           rsReplicaFailure,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:               ConditionReconciling,
				Status:             corev1.ConditionTrue,
				Reason:             "ReplicaFailure",
				LastTransitionTime: metav1.Time{},
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
			checkLastTransitionTime: true,
		},
		"rsReplicaFailureWithTransitionTime": {
			spec:           fmt.Sprintf(rsReplicaFailureWithTransitionTime, currentTimeStr),
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:               ConditionReconciling,
				Status:             corev1.ConditionTrue,
				Reason:             "ReplicaFailure",
				LastTransitionTime: metav1.Time{Time: currentTime},
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
			checkLastTransitionTime: true,
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var pdbNotObserved = `
apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
   generation: 2
   name: test
   namespace: qual
status:
   observedGeneration: 1
`

var pdbObserved = `
apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   observedGeneration: 1
`

func TestPDBStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"pdbNotObserved": {
			spec:           pdbNotObserved,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LatestGenerationNotObserved",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"pdbObserved": {
			spec:               pdbObserved,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var crdNoStatus = `
apiVersion: something/v1
kind: MyCR
metadata:
   generation: 1
   name: test
   namespace: qual
`

var crdMismatchStatusGeneration = `
apiVersion: something/v1
kind: MyCR
metadata:
   name: test
   namespace: qual
   generation: 2
status:
   observedGeneration: 1
`

var crdReady = `
apiVersion: something/v1
kind: MyCR
metadata:
   name: test
   namespace: qual
   generation: 1
status:
   conditions:
    - type: Ready 
      status: "True"
      message: All looks ok
      reason: AllOk
`

var crdNotReady = `
apiVersion: something/v1
kind: MyCR
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   observedGeneration: 1
   conditions:
    - type: Ready 
      status: "False"
      reason: NotReadyYet
`

var crdNoCondition = `
apiVersion: something/v1
kind: MyCR
metadata:
   name: test
   namespace: qual
   generation: 1
status:
   conditions:
    - type: SomeCondition 
      status: "False"
`

func TestCRDGenericStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"crdNoStatus": {
			spec:               crdNoStatus,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"crdReady": {
			spec:               crdReady,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"crdNotReady": {
			spec:           crdNotReady,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{
				{
					Type:   ConditionReconciling,
					Status: corev1.ConditionTrue,
					Reason: "NotReadyYet",
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"crdNoCondition": {
			spec:               crdNoCondition,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"crdMismatchStatusGeneration": {
			spec:           crdMismatchStatusGeneration,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "LatestGenerationNotObserved",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var jobNoStatus = `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   namespace: qual
   generation: 1
`

var jobComplete = `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   namespace: qual
   generation: 1
status:
   succeeded: 1
   active: 0
   conditions:
    - type: Complete 
      status: "True"
`

var jobFailed = `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   completions: 4
status:
   succeeded: 3
   failed: 1
   conditions:
    - type: Failed 
      status: "True"
      reason: JobFailed
`

var jobFailedWithLastTransitionTime = `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   completions: 4
status:
   succeeded: 3
   failed: 1
   conditions:
    - type: Failed 
      status: "True"
      reason: JobFailed
      lastTransitionTime: %s	  
`

var jobInProgress = `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
   completions: 10
   parallelism: 2
status:
   startTime: "2019-06-04T01:17:13Z"
   succeeded: 3
   failed: 1
   active: 2
   conditions:
    - type: Failed 
      status: "False"
    - type: Complete 
      status: "False"
`

func TestJobStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"jobNoStatus": {
			spec:           jobNoStatus,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "JobNotStarted",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"jobComplete": {
			spec:               jobComplete,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"jobFailed": {
			spec:           jobFailed,
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{{
				Type:               ConditionStalled,
				Status:             corev1.ConditionTrue,
				Reason:             "JobFailed",
				LastTransitionTime: metav1.Time{},
			}},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
		"jobFailedWithLastTransitionTime": {
			spec:           fmt.Sprintf(jobFailedWithLastTransitionTime, currentTimeStr),
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{{
				Type:               ConditionStalled,
				Status:             corev1.ConditionTrue,
				Reason:             "JobFailed",
				LastTransitionTime: metav1.Time{Time: currentTime},
			}},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
		"jobInProgress": {
			spec:               jobInProgress,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
				ConditionStalled,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var cronjobNoStatus = `
apiVersion: batch/v1
kind: CronJob
metadata:
   name: test
   namespace: qual
   generation: 1
`

var cronjobWithStatus = `
apiVersion: batch/v1
kind: CronJob
metadata:
   name: test
   namespace: qual
   generation: 1
status:
`

func TestCronJobStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"cronjobNoStatus": {
			spec:               cronjobNoStatus,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"cronjobWithStatus": {
			spec:               cronjobWithStatus,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var serviceDefault = `
apiVersion: v1
kind: Service
metadata:
   name: test
   namespace: qual
   generation: 1
`

var serviceNodePort = `
apiVersion: v1
kind: Service
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
  type: NodePort
`

var serviceLBok = `
apiVersion: v1
kind: Service
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
  type: LoadBalancer
  clusterIP: "1.2.3.4"
`
var serviceLBnok = `
apiVersion: v1
kind: Service
metadata:
   name: test
   namespace: qual
   generation: 1
spec:
  type: LoadBalancer
`

func TestServiceStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"serviceDefault": {
			spec:               serviceDefault,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"serviceNodePort": {
			spec:               serviceNodePort,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
		"serviceLBnok": {
			spec:           serviceLBnok,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{{
				Type:   ConditionReconciling,
				Status: corev1.ConditionTrue,
				Reason: "NoIPAssigned",
			}},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"serviceLBok": {
			spec:               serviceLBok,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
				ConditionReconciling,
			},
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var crdNoConditions = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
   generation: 1
`

var crdInstalling = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
   generation: 1
status:
   conditions:
    - type: NamesAccepted
      status: "True"
      reason: NoConflicts
    - type: Established
      status: "False"
      reason: Installing
`

var crdNamesNotAccepted = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
   generation: 1
status:
   conditions:
    - type: NamesAccepted
      status: "False"
      reason: SomeReason
`

var crdNamesNotAcceptedWithLastTransitionTime = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
   generation: 1
status:
   conditions:
    - type: NamesAccepted
      status: "False"
      reason: SomeReason
      lastTransitionTime: %s
`

var crdEstablished = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
   generation: 1
status:
   conditions:
    - type: NamesAccepted
      status: "True"
      reason: NoConflicts
    - type: Established
      status: "True"
      reason: InitialNamesAccepted
`

var crdNotEstablished = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
   generation: 1
status:
   conditions:
    - type: NamesAccepted
      status: "True"
      reason: NoConflicts
    - type: Established
      status: "False"
      reason: Unknown
`

var crdNotEstablishedWithLastTransitionTime = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
   generation: 1
status:
   conditions:
    - type: NamesAccepted
      status: "True"
      reason: NoConflicts
    - type: Established
      status: "False"
      reason: Unknown
      lastTransitionTime: %s
`

func TestCRDStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"crdNoConditions": {
			spec:           crdNoConditions,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{
				{
					Type:   ConditionReconciling,
					Status: corev1.ConditionTrue,
					Reason: "Installing",
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"crdInstalling": {
			spec:           crdInstalling,
			expectedStatus: InProgressStatus,
			expectedConditions: []Condition{
				{
					Type:   ConditionReconciling,
					Status: corev1.ConditionTrue,
					Reason: "Installing",
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionStalled,
			},
		},
		"crdNamesNotAccepted": {
			spec:           crdNamesNotAccepted,
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionStalled,
					Status:             corev1.ConditionTrue,
					Reason:             "SomeReason",
					LastTransitionTime: metav1.Time{},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
		"crdNamesNotAcceptedWithLastTransitionTime": {
			spec:           fmt.Sprintf(crdNamesNotAcceptedWithLastTransitionTime, currentTimeStr),
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionStalled,
					Status:             corev1.ConditionTrue,
					Reason:             "SomeReason",
					LastTransitionTime: metav1.Time{Time: currentTime},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
		"crdEstablished": {
			spec:               crdEstablished,
			expectedStatus:     CurrentStatus,
			expectedConditions: []Condition{},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
				ConditionStalled,
			},
		},
		"crdNotEstablished": {
			spec:           crdNotEstablished,
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionStalled,
					Status:             corev1.ConditionTrue,
					Reason:             "Unknown",
					LastTransitionTime: metav1.Time{},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},

		"crdNotEstablishedWithLastTransitionTime": {
			spec:           fmt.Sprintf(crdNotEstablishedWithLastTransitionTime, currentTimeStr),
			expectedStatus: FailedStatus,
			expectedConditions: []Condition{
				{
					Type:               ConditionStalled,
					Status:             corev1.ConditionTrue,
					Reason:             "Unknown",
					LastTransitionTime: metav1.Time{Time: currentTime},
				},
			},
			absentConditionTypes: []ConditionType{
				ConditionReconciling,
			},
			checkLastTransitionTime: true,
		},
	}

	for tn, tc := range testCases {
		tc := tc
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package status_test

import (
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/internal/pkg/clik8s"
	"sigs.k8s.io/cli-utils/internal/pkg/status"
	"sigs.k8s.io/cli-utils/internal/pkg/wirecli/wiretest"
)

func noitems() clik8s.ResourceConfigs {
	return clik8s.ResourceConfigs(nil)
}

func y2u(t *testing.T, spec string) *unstructured.Unstructured {
	j, err := yaml.YAMLToJSON([]byte(spec))
	assert.NoError(t, err)
	u, _, err := unstructured.UnstructuredJSONScheme.Decode(j, nil, nil)
	assert.NoError(t, err)
	return u.(*unstructured.Unstructured)
}

func TestEmptyStatus(t *testing.T) {
	a, done, err := wiretest.InitializeStatus()
	defer done()
	assert.NoError(t, err)
	r := a.FetchResourcesAndGetStatus(noitems())
	var expected []status.ResourceResult
	assert.Equal(t, expected, r)
}

type testSpec struct {
	spec string
	expectedStatus status.Status
	expectedConditions []status.Condition
	absentConditionTypes []status.ConditionType
}

func runStatusTest(t *testing.T, tc testSpec) {
	res, err := status.GetStatus(y2u(t, tc.spec))
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
   name: test
`

var podReady = `
apiVersion: v1
kind: Pod
metadata:
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
   name: test
   namespace: qual
status:
   phase: Failed
   conditions:
    - type: Ready 
      status: "False"
      reason: PodCompleted
`

// Test coverage using GetConditions
func TestPodStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"podNoStatus": {
			spec: podNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "PodNotReady",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"podReady": {
			spec: podReady,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionInProgress,
				status.ConditionFailed,
			},
		},
		"podCompletedSuccessfully": {
			spec: podCompletedOK,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionInProgress,
				status.ConditionFailed,
			},
		},
		"podCompletedFailed": {
			spec: podCompletedFail,
			expectedStatus: status.FailedStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionFailed,
				Status: corev1.ConditionTrue,
				Reason: "PodFailed",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionInProgress,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var pvcNoStatus = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
   name: test
`
var pvcBound = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
   name: test
   namespace: qual
status:
   phase: Bound
`

func TestPVCStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"pvcNoStatus": {
			spec: pvcNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "NotBound",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"pvcBound": {
			spec: pvcBound,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
	}

	for tn, tc := range testCases {
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

func TestStsStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"stsNoStatus": {
			spec: stsNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessReplicas",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"stsBadStatus": {
			spec: stsBadStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessReplicas",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"stsOK": {
			spec: stsOK,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"stsLessReady": {
			spec: stsLessReady,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessReady",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"stsLessCurrent": {
			spec: stsLessCurrent,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessCurrent",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
	}

	for tn, tc := range testCases {
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
			spec: dsNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "NoDesiredNumber",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"dsBadStatus": {
			spec: dsBadStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "NoDesiredNumber",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"dsOK": {
			spec: dsOK,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"dsLessReady": {
			spec: dsLessReady,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessReady",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"dsLessAvailable": {
			spec: dsLessAvailable,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessAvailable",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
	}

	for tn, tc := range testCases {
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

func TestDeploymentStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"depNoStatus": {
			spec: depNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessReplicas",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"depOK": {
			spec: depOK,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"depNotProgressing": {
			spec: depNotProgressing,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "ReplicaSetNotAvailable",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"depNotAvailable": {
			spec: depNotAvailable,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "DeploymentNotAvailable",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
	}

	for tn, tc := range testCases {
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
   labelledReplicas: 2
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
   labelledReplicas: 2
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
   labelledReplicas: 4
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
   labelledReplicas: 4
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
   labelledReplicas: 4
   availableReplicas: 4
   conditions:
    - type: ReplicaFailure 
      status: "True"
`

func TestReplicasetStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"rsNoStatus": {
			spec: rsNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessLabelled",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"rsOK1": {
			spec: rsOK1,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"rsOK2": {
			spec: rsOK2,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"rsLessAvailable": {
			spec: rsLessAvailable,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessAvailable",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"rsLessReady": {
			spec: rsLessReady,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LessReady",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"rsReplicaFailure": {
			spec: rsReplicaFailure,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "ReplicaFailure",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var pdbNoStatus = `
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
   name: test
`

var pdbOK1 = `
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
   name: test
   namespace: qual
status:
   currentHealthy: 2
   desiredHealthy: 2
`

var pdbMoreHealthy = `
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
   name: test
   namespace: qual
status:
   currentHealthy: 4
   desiredHealthy: 2
`

var pdbLessHealthy = `
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
   name: test
   namespace: qual
status:
   currentHealthy: 2
   desiredHealthy: 4
`

func TestPDBStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"pdbNoStatus": {
			spec: pdbNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "ZeroDesiredHealthy",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"pdbOK1": {
			spec: pdbOK1,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"pdbMoreHealthy": {
			spec: pdbMoreHealthy,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"pdbLessHealthy": {
			spec: pdbLessHealthy,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "BudgetNotMet",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

var crdNoStatus = `
apiVersion: something/v1
kind: MyCR
metadata:
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
`

var crdNoCondition = `
apiVersion: something/v1
kind: MyCR
metadata:
   name: test
   namespace: qual
status:
   conditions:
    - type: SomeCondition 
      status: "False"
`

func TestCRDGenericStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"crdNoStatus": {
			spec: crdNoStatus,
			expectedStatus: status.UnknownStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"crdReady": {
			spec: crdReady,
			expectedStatus: status.UnknownStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"crdNotReady": {
			spec: crdNotReady,
			expectedStatus: status.UnknownStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"crdNoCondition": {
			spec: crdNoCondition,
			expectedStatus: status.UnknownStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"crdMismatchStatusGeneration": {
			spec: crdMismatchStatusGeneration,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "LatestGenerationNotObserved",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
	}

	for tn, tc := range testCases {
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
`

var jobComplete = `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   namespace: qual
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
spec:
   completions: 4
status:
   succeeded: 3
   failed: 1
   conditions:
    - type: Failed 
      status: "True"
`

var jobInProgress = `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   namespace: qual
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
			spec: jobNoStatus,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "JobNotStarted",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"jobComplete": {
			spec: jobComplete,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"jobFailed": {
			spec: jobFailed,
			expectedStatus: status.FailedStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionFailed,
				Status: corev1.ConditionTrue,
				Reason: "JobFailed",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionInProgress,
			},
		},
		"jobInProgress": {
			spec: jobInProgress,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionInProgress,
				status.ConditionFailed,
			},
		},
	}

	for tn, tc := range testCases {
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
`

var cronjobWithStatus = `
apiVersion: batch/v1
kind: CronJob
metadata:
   name: test
   namespace: qual
status:
`

func TestCronJobStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"cronjobNoStatus": {
			spec: cronjobNoStatus,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"cronjobWithStatus": {
			spec: cronjobWithStatus,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
	}

	for tn, tc := range testCases {
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
`

var serviceNodePort = `
apiVersion: v1
kind: Service
metadata:
   name: test
   namespace: qual
spec:
  type: NodePort
`

var serviceLBok = `
apiVersion: v1
kind: Service
metadata:
   name: test
   namespace: qual
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
spec:
  type: LoadBalancer
`

func TestServiceStatus(t *testing.T) {
	testCases := map[string]testSpec{
		"serviceDefault": {
			spec: serviceDefault,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"serviceNodePort": {
			spec: serviceNodePort,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
		"serviceLBnok": {
			spec: serviceLBnok,
			expectedStatus: status.InProgressStatus,
			expectedConditions: []status.Condition{{
				Type: status.ConditionInProgress,
				Status: corev1.ConditionTrue,
				Reason: "NoIPAssigned",
			}},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
			},
		},
		"serviceLBok": {
			spec: serviceLBok,
			expectedStatus: status.CurrentStatus,
			expectedConditions: []status.Condition{},
			absentConditionTypes: []status.ConditionType{
				status.ConditionFailed,
				status.ConditionInProgress,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			runStatusTest(t, tc)
		})
	}
}

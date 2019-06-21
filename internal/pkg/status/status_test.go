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
	"bytes"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/status"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
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
	buf := new(bytes.Buffer)
	a, done, err := wiretest.InitializeStatus(noitems(), &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r := a.Do()
	assert.Equal(t, status.Result{Resources: []status.ResourceStatus{}}, r)
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
	r, err := status.GetConditions(y2u(t, podNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Phase: unknown", ready.Message)
	condition := status.GetCondition(r, status.ConditionCompleted)
	assert.Equal(t, (*status.Condition)(nil), condition)
	condition = status.GetCondition(r, status.ConditionFailed)
	assert.Equal(t, (*status.Condition)(nil), condition)

	r, err = status.GetConditions(y2u(t, podReady))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Phase: Running", ready.Message)
	condition = status.GetCondition(r, status.ConditionCompleted)
	assert.Equal(t, (*status.Condition)(nil), condition)
	condition = status.GetCondition(r, status.ConditionFailed)
	assert.Equal(t, (*status.Condition)(nil), condition)

	r, err = status.GetConditions(y2u(t, podCompletedOK))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Phase: Succeeded, PodCompleted", ready.Message)
	condition = status.GetCondition(r, status.ConditionCompleted)
	assert.NotEqual(t, nil, condition)
	assert.Equal(t, "True", condition.Status)
	assert.Equal(t, "Pod Succeeded", condition.Message)
	condition = status.GetCondition(r, status.ConditionFailed)
	assert.Equal(t, (*status.Condition)(nil), condition)

	r, err = status.GetConditions(y2u(t, podCompletedFail))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Phase: Failed, PodCompleted", ready.Message)
	condition = status.GetCondition(r, status.ConditionCompleted)
	assert.Equal(t, (*status.Condition)(nil), condition)
	condition = status.GetCondition(r, status.ConditionFailed)
	assert.NotEqual(t, nil, condition)
	assert.Equal(t, "True", condition.Status)
	assert.Equal(t, "Pod phase: Failed", condition.Message)
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

var pvcUnBound = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
   name: test
   namespace: qual
status:
   phase: UnBound
`

func TestPVCStatus(t *testing.T) {
	r, err := status.GetConditions(y2u(t, pvcNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "PVC is not Bound. phase: unknown", ready.Message)

	r, err = status.GetConditions(y2u(t, pvcBound))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "PVC is Bound", ready.Message)

	r, err = status.GetConditions(y2u(t, pvcUnBound))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "PVC is not Bound. phase: UnBound", ready.Message)
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
	r, err := status.GetConditions(y2u(t, stsNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Controller has not observed the latest change. Status generation does not match with metadata", ready.Message)
	assert.Equal(t, "NotObserved", ready.Reason)

	r, err = status.GetConditions(y2u(t, stsBadStatus))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "LessReplicas", ready.Reason)
	assert.Equal(t, "Waiting for requested replicas. Replicas: 0/1", ready.Message)

	r, err = status.GetConditions(y2u(t, stsOK))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "ReplicasOK", ready.Reason)
	assert.Equal(t, "All replicas scheduled as expected. Replicas: 4", ready.Message)

	r, err = status.GetConditions(y2u(t, stsLessReady))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "LessReady", ready.Reason)
	assert.Equal(t, "Waiting for replicas to become Ready. Ready: 2/4", ready.Message)

	r, err = status.GetConditions(y2u(t, stsLessCurrent))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "LessCurrent", ready.Reason)
	assert.Equal(t, "Waiting for replicas to become current. current: 2/4", ready.Message)
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
	r, err := status.GetConditions(y2u(t, dsNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Controller has not observed the latest change. Status generation does not match with metadata", ready.Message)

	r, err = status.GetConditions(y2u(t, dsBadStatus))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Missing .status.desiredNumberScheduled", ready.Message)

	r, err = status.GetConditions(y2u(t, dsOK))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "All replicas scheduled as expected. Replicas: 4", ready.Message)

	r, err = status.GetConditions(y2u(t, dsLessReady))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Waiting for replicas to be ready. Ready: 2/4", ready.Message)

	r, err = status.GetConditions(y2u(t, dsLessAvailable))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Waiting for replicas to be available. Available: 2/4", ready.Message)
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
	r, err := status.GetConditions(y2u(t, depNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Controller has not observed the latest change. Status generation does not match with metadata", ready.Message)
	assert.Equal(t, "NotObserved", ready.Reason)

	r, err = status.GetConditions(y2u(t, depOK))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "ReplicasOK", ready.Reason)
	assert.Equal(t, "Deployment is available. Replicas: 1", ready.Message)

	r, err = status.GetConditions(y2u(t, depNotProgressing))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "ReplicaSetNotAvailable", ready.Reason)
	assert.Equal(t, "ReplicaSet not Available", ready.Message)

	r, err = status.GetConditions(y2u(t, depNotAvailable))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "DeploymentNotAvailable", ready.Reason)
	assert.Equal(t, "Deployment not Available", ready.Message)
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
	r, err := status.GetConditions(y2u(t, rsNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Controller has not observed the latest change. Status generation does not match with metadata", ready.Message)

	r, err = status.GetConditions(y2u(t, rsOK1))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "ReplicaSet is available. Replicas: 2", ready.Message)

	r, err = status.GetConditions(y2u(t, rsOK2))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "ReplicaSet is available. Replicas: 2", ready.Message)

	r, err = status.GetConditions(y2u(t, rsLessAvailable))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Waiting for all replicas to be available. Available: 2/4", ready.Message)

	r, err = status.GetConditions(y2u(t, rsLessReady))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Waiting for all replicas to be ready. Ready: 2/4", ready.Message)

	r, err = status.GetConditions(y2u(t, rsReplicaFailure))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Replica Failure condition. Check Pods", ready.Message)
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
	r, err := status.GetConditions(y2u(t, pdbNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Missing or zero .status.desiredHealthy", ready.Reason)

	r, err = status.GetConditions(y2u(t, pdbOK1))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Budget is met. Replicas: 2/2", ready.Reason)

	r, err = status.GetConditions(y2u(t, pdbMoreHealthy))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Budget is met. Replicas: 4/2", ready.Reason)

	r, err = status.GetConditions(y2u(t, pdbLessHealthy))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Budget not met. healthy replicas: 2/4", ready.Reason)
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
	r, err := status.GetConditions(y2u(t, crdNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "No Ready condition found in status", ready.Message)

	r, err = status.GetConditions(y2u(t, crdReady))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "All looks ok", ready.Message)

	r, err = status.GetConditions(y2u(t, crdNotReady))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "", ready.Message)

	r, err = status.GetConditions(y2u(t, crdNoCondition))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "No Ready condition found in status", ready.Message)

	r, err = status.GetConditions(y2u(t, crdMismatchStatusGeneration))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Controller has not observed the latest change. Status generation does not match with metadata", ready.Message)
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
	r, err := status.GetConditions(y2u(t, jobNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "Job not started", ready.Message)
	condition := status.GetCondition(r, status.ConditionFailed)
	assert.Equal(t, (*status.Condition)(nil), condition)

	r, err = status.GetConditions(y2u(t, jobComplete))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Job Completed. succeded: 1/1", ready.Message)
	condition = status.GetCondition(r, status.ConditionCompleted)
	assert.NotEqual(t, (*status.Condition)(nil), condition)
	assert.Equal(t, "True", condition.Status)
	assert.Equal(t, "Job Completed. succeded: 1/1", condition.Message)

	r, err = status.GetConditions(y2u(t, jobFailed))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Job Failed. failed: 1/4", ready.Message)
	condition = status.GetCondition(r, status.ConditionFailed)
	assert.NotEqual(t, (*status.Condition)(nil), condition)
	assert.Equal(t, "True", condition.Status)
	assert.Equal(t, "Job Failed. failed: 1/4", condition.Message)

	r, err = status.GetConditions(y2u(t, jobInProgress))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Job in progress. success:3, active: 2, failed: 1", ready.Message)
	condition = status.GetCondition(r, status.ConditionFailed)
	assert.Equal(t, (*status.Condition)(nil), condition)
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
	r, err := status.GetConditions(y2u(t, cronjobNoStatus))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Always", ready.Reason)

	r, err = status.GetConditions(y2u(t, cronjobWithStatus))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Always", ready.Reason)
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
	r, err := status.GetConditions(y2u(t, serviceDefault))
	assert.NoError(t, err)
	ready := status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Always Ready. Service type: ClusterIP", ready.Message)

	r, err = status.GetConditions(y2u(t, serviceNodePort))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "Always Ready. Service type: NodePort", ready.Message)

	r, err = status.GetConditions(y2u(t, serviceLBnok))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "False", ready.Status)
	assert.Equal(t, "ClusterIP not set. Service type: LoadBalancer", ready.Message)

	r, err = status.GetConditions(y2u(t, serviceLBok))
	assert.NoError(t, err)
	ready = status.GetCondition(r, status.ConditionReady)
	assert.NotEqual(t, nil, ready)
	assert.Equal(t, "True", ready.Status)
	assert.Equal(t, "ClusterIP: 1.2.3.4", ready.Message)
}

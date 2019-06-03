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
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, status.Result{Ready: true, Resources: []status.ResourceStatus{}}, r)
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

// Test coverage using IsReady
func TestPodStatus(t *testing.T) {
	r, err := status.IsReady(y2u(t, podNoStatus))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	for _, spec := range []string{podReady, podCompletedOK, podCompletedFail} {
		r, err = status.IsReady(y2u(t, spec))
		assert.NoError(t, err)
		assert.Equal(t, true, r)
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
	r, err := status.IsReady(y2u(t, pvcNoStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, pvcBound))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, pvcUnBound))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
}

var stsNoStatus = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   name: test
`
var stsBadStatus = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   name: test
   namespace: qual
status:
   currentReplicas: 1
`

var stsOK = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   name: test
   namespace: qual
spec:
   replicas: 4
status:
   currentReplicas: 4
   readyReplicas: 4
`

var stsLessReady = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   name: test
   namespace: qual
spec:
   replicas: 4
status:
   currentReplicas: 4
   readyReplicas: 2
`
var stsLessCurrent = `
apiVersion: apps/v1
kind: StatefulSet
metadata:
   name: test
   namespace: qual
spec:
   replicas: 4
status:
   currentReplicas: 2
   readyReplicas: 4
`

func TestStsStatus(t *testing.T) {
	r, err := status.IsReady(y2u(t, stsNoStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, stsBadStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, stsOK))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, stsLessReady))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, stsLessCurrent))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
}

var dsNoStatus = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
`
var dsBadStatus = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
status:
   currentReplicas: 1
`

var dsOK = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
status:
   desiredNumberScheduled: 4
   numberAvailable: 4
   numberReady: 4
`

var dsLessReady = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
status:
   desiredNumberScheduled: 4
   numberAvailable: 4
   numberReady: 2
`
var dsLessAvailable = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
   name: test
   namespace: qual
status:
   desiredNumberScheduled: 4
   numberAvailable: 2
   numberReady: 4
`

func TestDaemonsetStatus(t *testing.T) {
	r, err := status.IsReady(y2u(t, dsNoStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, dsBadStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, dsOK))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, dsLessReady))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, dsLessAvailable))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
}

var depNoStatus = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
`

var depOK = `
apiVersion: apps/v1
kind: Deployment
metadata:
   name: test
   namespace: qual
status:
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
   namespace: qual
status:
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
   namespace: qual
status:
   conditions:
    - type: Progressing 
      status: "True"
      reason: NewReplicaSetAvailable
    - type: Available 
      status: "False"
`

func TestDeploymentStatus(t *testing.T) {
	r, err := status.IsReady(y2u(t, depNoStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, depOK))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, depNotProgressing))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, depNotAvailable))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
}

var rsNoStatus = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
`

var rsOK1 = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
status:
   replicas: 2
   readyReplicas: 2
   availableReplicas: 2
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
status:
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
status:
   replicas: 4
   readyReplicas: 2
   availableReplicas: 4
`

var rsLessAvailable = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
status:
   replicas: 4
   readyReplicas: 4
   availableReplicas: 2
`

var rsReplicaFailure = `
apiVersion: apps/v1
kind: ReplicaSet
metadata:
   name: test
   namespace: qual
status:
   replicas: 4
   readyReplicas: 4
   availableReplicas: 4
   conditions:
    - type: ReplicaFailure 
      status: "True"
`

func TestReplicasetStatus(t *testing.T) {
	r, err := status.IsReady(y2u(t, rsNoStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, rsOK1))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, rsOK2))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, rsLessAvailable))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, rsLessReady))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, rsReplicaFailure))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
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
	r, err := status.IsReady(y2u(t, pdbNoStatus))
	assert.Error(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, pdbOK1))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, pdbMoreHealthy))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, pdbLessHealthy))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
}

var crdNoStatus = `
apiVersion: something/v1
kind: MyCR
metadata:
   name: test
   namespace: qual
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
`

var crdNotReady = `
apiVersion: something/v1
kind: MyCR
metadata:
   name: test
   namespace: qual
status:
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
	r, err := status.IsReady(y2u(t, crdNoStatus))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, crdReady))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, crdNotReady))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, crdNoCondition))
	assert.NoError(t, err)
	assert.Equal(t, true, r)
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
status:
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
status:
   conditions:
    - type: Failed 
      status: "False"
    - type: Complete 
      status: "False"
`

func TestJobStatus(t *testing.T) {
	r, err := status.IsReady(y2u(t, jobNoStatus))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, jobComplete))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, jobFailed))
	assert.NoError(t, err)
	assert.Equal(t, true, r)

	r, err = status.IsReady(y2u(t, jobInProgress))
	assert.NoError(t, err)
	assert.Equal(t, false, r)
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
	r, err := status.IsReady(y2u(t, cronjobNoStatus))
	assert.NoError(t, err)
	assert.Equal(t, false, r)

	r, err = status.IsReady(y2u(t, cronjobWithStatus))
	assert.NoError(t, err)
	assert.Equal(t, true, r)
}

// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/yaml"
)

func TestExampleCompute(t *testing.T) {
	deploymentManifest := `
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
	deployment := testutil.YamlToUnstructured(t, deploymentManifest)

	res, err := status.Compute(deployment)
	assert.NoError(t, err)

	assert.Equal(t, status.Status("Current"), res.Status)
}

func TestExampleAugment(t *testing.T) {
	deploymentManifest := `
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
	deployment := testutil.YamlToUnstructured(t, deploymentManifest)

	err := status.Augment(deployment)
	assert.NoError(t, err)

	b, err := yaml.Marshal(deployment.Object)
	assert.NoError(t, err)

	receivedManifest := strings.TrimSpace(string(b))
	expectedManifest := strings.TrimSpace(`
apiVersion: apps/v1
kind: Deployment
metadata:
  generation: 1
  name: test
  namespace: qual
status:
  availableReplicas: 1
  conditions:
  - reason: NewReplicaSetAvailable
    status: "True"
    type: Progressing
  - status: "True"
    type: Available
  observedGeneration: 1
  readyReplicas: 1
  replicas: 1
  updatedReplicas: 1
`)

	assert.Equal(t, expectedManifest, receivedManifest)
}

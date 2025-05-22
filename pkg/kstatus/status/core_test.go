// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// Tests are based on https://kubernetes.io/docs/concepts/workloads/controllers/job/#suspending-a-job.

// Validate that a job with spec.suspend set to true and a condition of type
// Suspended with status True is considered Current.
func TestSuspendedJobConditions(t *testing.T) {
	jobManifest := `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   generation: 1
   namespace: qual
spec:
  suspend: true
status:
  conditions:
  - type: Suspended
    status: "True"
`

	suspendedJob := testutil.YamlToUnstructured(t, jobManifest)

	res, err := status.Compute(suspendedJob)
	assert.NoError(t, err)

	assert.Equal(t, status.Status("Current"), res.Status)
}

// Validate that a job previously suspended shows as in-progress if the
// spec.suspend is set to false.
func TestPreviouslySuspendedJobConditions(t *testing.T) {
	jobManifest := `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   generation: 1
   namespace: qual
spec:
  suspend: false
status:
  conditions:
  - type: Suspended
    status: "False"
`

	suspendedJob := testutil.YamlToUnstructured(t, jobManifest)

	res, err := status.Compute(suspendedJob)
	assert.NoError(t, err)

	assert.Equal(t, status.Status("InProgress"), res.Status)
}

// Validate that a job that is transitioning to or from the suspended state
// will show as in-progress.
func TestSuspendedJobTransitionConditions(t *testing.T) {
	jobManifest := `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   generation: 1
   namespace: qual
spec:
  suspend: %t
status:
  conditions:
  - type: Suspended
    status: "%s"
`

	toSuspendedStateManifest := fmt.Sprintf(jobManifest, true, "False")
	fromSuspendedStateManifest := fmt.Sprintf(jobManifest, false, "True")

	toSuspendedJob := testutil.YamlToUnstructured(t, toSuspendedStateManifest)
	fromSuspendedJob := testutil.YamlToUnstructured(t, fromSuspendedStateManifest)

	res, err := status.Compute(toSuspendedJob)
	assert.NoError(t, err)

	assert.Equal(t, status.Status("InProgress"), res.Status)

	res, err = status.Compute(fromSuspendedJob)
	assert.NoError(t, err)

	assert.Equal(t, status.Status("InProgress"), res.Status)
}

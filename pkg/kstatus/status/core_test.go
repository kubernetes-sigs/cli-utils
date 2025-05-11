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

func TestSuspendedJobConditions(t *testing.T) {
	jobManifest := `
apiVersion: batch/v1
kind: Job
metadata:
   name: test
   generation: 1
   namespace: qual
status:
  conditions:
  - type: Suspended
    status: "%s"
    reason: JobSuspended
`

	suspendedJobManifest := fmt.Sprintf(jobManifest, "True")
	inprogressJobManifest := fmt.Sprintf(jobManifest, "False")

	suspendedJob := testutil.YamlToUnstructured(t, suspendedJobManifest)
	inprogressJob := testutil.YamlToUnstructured(t, inprogressJobManifest)

	res, err := status.Compute(suspendedJob)
	assert.NoError(t, err)

	assert.Equal(t, status.Status("Current"), res.Status)

	res, err = status.Compute(inprogressJob)
	assert.NoError(t, err)

	assert.Equal(t, status.Status("InProgress"), res.Status)
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestTextForError(t *testing.T) {
	testCases := map[string]struct {
		err             error
		cmdNameBase     string
		expectFound     bool
		expectedErrText string
	}{
		"known error without directives in the template": {
			err:             inventory.MultipleInventoryObjError{},
			cmdNameBase:     "kapply",
			expectFound:     true,
			expectedErrText: "Package has multiple inventory object templates.",
		},
		"unknown error": {
			err:         fmt.Errorf("this is a test"),
			cmdNameBase: "kapply",
			expectFound: false,
		},
		"unknown error type": {
			err:         sliceError{},
			cmdNameBase: "kapply",
			expectFound: false,
		},
		"timeout error": {
			err: &taskrunner.TimeoutError{
				Timeout: 2 * time.Second,
				Identifiers: []object.ObjMetadata{
					{
						GroupKind: schema.GroupKind{
							Kind:  "Deployment",
							Group: "apps",
						},
						Name: "foo",
					},
				},
				Condition: taskrunner.AllCurrent,
				TimedOutResources: []taskrunner.TimedOutResource{
					{
						Identifier: object.ObjMetadata{
							GroupKind: schema.GroupKind{
								Kind:  "Deployment",
								Group: "apps",
							},
							Name: "foo",
						},
						Status: status.InProgressStatus,
					},
				},
			},
			cmdNameBase: "kapply",
			expectFound: true,
			expectedErrText: `
Timeout after 2 seconds waiting for 1 out of 1 resources to reach condition AllCurrent:
Deployment/foo InProgress
`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			errText, found := textForError(tc.err, tc.cmdNameBase)

			if !tc.expectFound {
				assert.False(t, found)
				return
			}

			assert.True(t, found)
			assert.Contains(t, errText, strings.TrimSpace(tc.expectedErrText))
		})
	}
}

type sliceError []string

func (s sliceError) Error() string {
	return "this is a test"
}

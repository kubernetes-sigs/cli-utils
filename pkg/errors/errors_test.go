// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

func TestTextForError(t *testing.T) {
	testCases := map[string]struct {
		err             error
		cmdNameBase     string
		expectFound     bool
		expectedErrText string
	}{
		"kapply command base name": {
			err:             inventory.NoInventoryObjError{},
			cmdNameBase:     "kapply",
			expectFound:     true,
			expectedErrText: "Please run \"kapply init\" command.",
		},
		"different command base name": {
			err:             inventory.NoInventoryObjError{},
			cmdNameBase:     "mycommand",
			expectFound:     true,
			expectedErrText: "Please run \"mycommand init\" command.",
		},
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

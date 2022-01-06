// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvalidAnnotationErrorString(t *testing.T) {
	testCases := map[string]struct {
		err            InvalidAnnotationError
		expectedString string
	}{
		"cluster-scoped": {
			err: InvalidAnnotationError{
				Annotation: "example",
				Cause:      errors.New("underlying error"),
			},
			expectedString: `invalid "example" annotation: underlying error`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, tc.expectedString, tc.err.Error())
		})
	}
}

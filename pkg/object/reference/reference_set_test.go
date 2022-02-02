// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package reference

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseObjectReferenceSet(t *testing.T) {
	testCases := map[string]struct {
		input         string
		expected      ObjectReferenceSet
		expectedError error
	}{
		"empty annotation is error": {
			input:         "",
			expected:      ObjectReferenceSet{},
			expectedError: errors.New(`failed to parse object reference (index: 0): wrong number of segments: ""`),
		},

		"missing namespace prefix": {
			input: "test-group/test-namespace/test-kind/namespaced-obj",
			expected: ObjectReferenceSet{
				{
					// version can be anything, so this is allowed,
					// but will probably error later when looking up the object.
					APIVersion: "test-group/test-namespace",
					Kind:       "test-kind",
					Name:       "namespaced-obj",
				},
			},
		},
		"missing namespace": {
			input: "test-group/namespaces/test-kind/cluster-obj",
			expected: ObjectReferenceSet{
				{
					// version can be anything, so this is allowed,
					// but will probably error later when looking up the object.
					APIVersion: "test-group/namespaces",
					Kind:       "test-kind",
					Name:       "cluster-obj",
				},
			},
		},
		"cluster-scoped object annotation": {
			input:    "test-group/test-kind/cluster-obj",
			expected: ObjectReferenceSet{clusterScopedObjRef},
		},
		"namespaced object annotation": {
			input:    "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			expected: ObjectReferenceSet{namespacedObjRef},
		},
		"namespaced object annotation with whitespace at ends is valid": {
			input:    "  test-group/namespaces/test-namespace/test-kind/namespaced-obj\n",
			expected: ObjectReferenceSet{namespacedObjRef},
		},
		"multiple object annotation": {
			input: "test-group/namespaces/test-namespace/test-kind/namespaced-obj," +
				"test-group/test-kind/cluster-obj",
			expected: ObjectReferenceSet{namespacedObjRef, clusterScopedObjRef},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ParseObjectReferenceSet(tc.input)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestObjectReferenceSetString(t *testing.T) {
	testCases := map[string]struct {
		objRefs  ObjectReferenceSet
		expected string
	}{
		"empty set is not error": {
			objRefs:  ObjectReferenceSet{},
			expected: "",
		},
		"cluster-scoped": {
			objRefs:  ObjectReferenceSet{clusterScopedObjRef},
			expected: "test-group/test-kind/cluster-obj",
		},
		"namespace-scoped": {
			objRefs:  ObjectReferenceSet{namespacedObjRef},
			expected: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
		},
		"multiple dependencies": {
			objRefs: ObjectReferenceSet{clusterScopedObjRef, namespacedObjRef},
			expected: "test-group/test-kind/cluster-obj," +
				"test-group/namespaces/test-namespace/test-kind/namespaced-obj",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual := tc.objRefs.String()
			assert.Equal(t, tc.expected, actual)
		})
	}
}

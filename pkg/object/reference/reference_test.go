// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package reference

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var clusterScopedObjRef = ObjectReference{
	Group: "test-group",
	Kind:  "test-kind",
	Name:  "cluster-obj",
}

var clusterScopedVersionedObjRef = ObjectReference{
	APIVersion: "test-group/v1",
	Kind:       "test-kind",
	Name:       "cluster-obj",
}

var namespacedObjRef = ObjectReference{
	Group:     "test-group",
	Kind:      "test-kind",
	Name:      "namespaced-obj",
	Namespace: "test-namespace",
}

var namespacedVersionedObjRef = ObjectReference{
	APIVersion: "test-group/v1",
	Kind:       "test-kind",
	Name:       "namespaced-obj",
	Namespace:  "test-namespace",
}

func TestParseObjectReference(t *testing.T) {
	testCases := map[string]struct {
		metaStr       string
		expected      ObjectReference
		expectedError error
	}{
		"empty is error": {
			metaStr:       "",
			expectedError: errors.New(`wrong number of segments: ""`),
		},
		"missing namespace prefix": {
			metaStr: "test-group/test-namespace/test-kind/namespaced-obj",
			expected: ObjectReference{
				// version can be anything, so this is allowed,
				// but will probably error later when looking up the object.
				APIVersion: "test-group/test-namespace",
				Kind:       "test-kind",
				Name:       "namespaced-obj",
			},
		},
		"missing namespace": {
			metaStr: "test-group/namespaces/test-kind/cluster-obj",
			expected: ObjectReference{
				// version can be anything, so this is allowed,
				// but will probably error later when looking up the object.
				APIVersion: "test-group/namespaces",
				Kind:       "test-kind",
				Name:       "cluster-obj",
			},
		},
		"cluster-scoped object": {
			metaStr:  "test-group/test-kind/cluster-obj",
			expected: clusterScopedObjRef,
		},
		"namespace-scoped": {
			metaStr:  "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			expected: namespacedObjRef,
		},
		"namespace-scoped with trailing whitespace is valid": {
			metaStr:  "  test-group/namespaces/test-namespace/test-kind/namespaced-obj\n",
			expected: namespacedObjRef,
		},
		"multiple is error": {
			metaStr: "test-group/namespaces/test-namespace/test-kind/namespaced-obj," +
				"test-group/test-kind/cluster-obj",
			expectedError: errors.New(`wrong number of segments: "test-group/namespaces/test-namespace/test-kind/namespaced-obj,test-group/test-kind/cluster-obj"`),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ParseObjectReference(tc.metaStr)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestObjectReferenceString(t *testing.T) {
	testCases := map[string]struct {
		objMeta  ObjectReference
		expected string
	}{
		"empty": {
			objMeta:  ObjectReference{},
			expected: "//",
		},
		"cluster-scoped": {
			objMeta:  clusterScopedObjRef,
			expected: "test-group/test-kind/cluster-obj",
		},
		"cluster-scoped with version": {
			objMeta:  clusterScopedVersionedObjRef,
			expected: "test-group/v1/test-kind/cluster-obj",
		},
		"namespace-scoped": {
			objMeta:  namespacedObjRef,
			expected: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
		},
		"namespace-scoped with version": {
			objMeta:  namespacedVersionedObjRef,
			expected: "test-group/v1/namespaces/test-namespace/test-kind/namespaced-obj",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual := tc.objMeta.String()
			assert.Equal(t, tc.expected, actual)
		})
	}
}

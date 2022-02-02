// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package dependents

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/object/reference"
)

var u1 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "unused",
			"namespace": "unused",
			"annotations": map[string]interface{}{
				Annotation: "test-group/test-kind/cluster-obj",
			},
		},
	},
}

var u2 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "unused",
			"namespace": "unused",
			"annotations": map[string]interface{}{
				Annotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			},
		},
	},
}

var multipleAnnotations = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "unused",
			"namespace": "unused",
			"annotations": map[string]interface{}{
				Annotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj," +
					"test-group/test-kind/cluster-obj",
			},
		},
	},
}

var noAnnotations = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "unused",
			"namespace": "unused",
		},
	},
}

var badAnnotation = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "unused",
			"namespace": "unused",
			"annotations": map[string]interface{}{
				Annotation: "test-group:namespaces:test-namespace:test-kind:namespaced-obj",
			},
		},
	},
}

var clusterScopedObj = reference.ObjectReference{
	Name:  "cluster-obj",
	Group: "test-group",
	Kind:  "test-kind",
}

var namespacedObj = reference.ObjectReference{
	Namespace: "test-namespace",
	Name:      "namespaced-obj",
	Group:     "test-group",
	Kind:      "test-kind",
}

func TestReadAnnotation(t *testing.T) {
	testCases := map[string]struct {
		obj           *unstructured.Unstructured
		expected      DependentSet
		expectedError error
	}{
		"nil object is not found": {
			obj:      nil,
			expected: DependentSet{},
		},
		"Object with no annotations returns not found": {
			obj:      noAnnotations,
			expected: DependentSet{},
		},
		"Unparseable depends on annotation returns not found": {
			obj:      badAnnotation,
			expected: DependentSet{},
			expectedError: errors.New(`invalid "config.kubernetes.io/dependents" annotation: ` +
				`failed to parse object reference (index: 0): ` +
				`wrong number of segments: ` +
				`"test-group:namespaces:test-namespace:test-kind:namespaced-obj"`),
		},
		"Cluster-scoped object depends on annotation": {
			obj:      u1,
			expected: DependentSet{clusterScopedObj},
		},
		"Namespaced object depends on annotation": {
			obj:      u2,
			expected: DependentSet{namespacedObj},
		},
		"Multiple objects specified in annotation": {
			obj:      multipleAnnotations,
			expected: DependentSet{namespacedObj, clusterScopedObj},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ReadAnnotation(tc.obj)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// getDependentsAnnotation wraps the depends-on annotation with a pointer.
// Returns nil if the annotation is missing.
func getDependentsAnnotation(obj *unstructured.Unstructured) *string {
	value, found := obj.GetAnnotations()[Annotation]
	if !found {
		return nil
	}
	return &value
}

func TestWriteAnnotation(t *testing.T) {
	testCases := map[string]struct {
		obj           *unstructured.Unstructured
		dependson     DependentSet
		expected      *string
		expectedError error
	}{
		"nil object": {
			obj:           nil,
			dependson:     DependentSet{},
			expected:      nil,
			expectedError: errors.New("object is nil"),
		},
		"empty mutation": {
			obj:           &unstructured.Unstructured{},
			dependson:     DependentSet{},
			expected:      nil,
			expectedError: errors.New("dependent set is empty"),
		},
		"Namespace-scoped object": {
			obj:       &unstructured.Unstructured{},
			dependson: DependentSet{namespacedObj},
			expected:  getDependentsAnnotation(u2),
		},
		"Cluster-scoped object": {
			obj:       &unstructured.Unstructured{},
			dependson: DependentSet{clusterScopedObj},
			expected:  getDependentsAnnotation(u1),
		},
		"Multiple objects": {
			obj:       &unstructured.Unstructured{},
			dependson: DependentSet{namespacedObj, clusterScopedObj},
			expected:  getDependentsAnnotation(multipleAnnotations),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			err := WriteAnnotation(tc.obj, tc.dependson)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, getDependentsAnnotation(tc.obj))
		})
	}
}

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package dependson

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

func TestReadAnnotation(t *testing.T) {
	testCases := map[string]struct {
		obj      *unstructured.Unstructured
		expected DependencySet
		isError  bool
	}{
		"nil object is not found": {
			obj:      nil,
			expected: DependencySet{},
		},
		"Object with no annotations returns not found": {
			obj:      noAnnotations,
			expected: DependencySet{},
		},
		"Unparseable depends on annotation returns not found": {
			obj:      badAnnotation,
			expected: DependencySet{},
			isError:  true,
		},
		"Cluster-scoped object depends on annotation": {
			obj:      u1,
			expected: DependencySet{clusterScopedObj},
		},
		"Namespaced object depends on annotation": {
			obj:      u2,
			expected: DependencySet{namespacedObj},
		},
		"Multiple objects specified in annotation": {
			obj:      multipleAnnotations,
			expected: DependencySet{namespacedObj, clusterScopedObj},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ReadAnnotation(tc.obj)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error not received")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
				if !actual.Equal(tc.expected) {
					t.Errorf("expected (%s), got (%s)", tc.expected, actual)
				}
			}
		})
	}
}

// getDependsOnAnnotation wraps the depends-on annotation with a pointer.
// Returns nil if the annotation is missing.
func getDependsOnAnnotation(obj *unstructured.Unstructured) *string {
	value, found := obj.GetAnnotations()[Annotation]
	if !found {
		return nil
	}
	return &value
}

func TestWriteAnnotation(t *testing.T) {
	testCases := map[string]struct {
		obj       *unstructured.Unstructured
		dependson DependencySet
		expected  *string
		isError   bool
	}{
		"nil object": {
			obj:       nil,
			dependson: DependencySet{},
			expected:  nil,
			isError:   true,
		},
		"empty mutation": {
			obj:       &unstructured.Unstructured{},
			dependson: DependencySet{},
			expected:  nil,
			isError:   true,
		},
		"Namespace-scoped object": {
			obj:       &unstructured.Unstructured{},
			dependson: DependencySet{namespacedObj},
			expected:  getDependsOnAnnotation(u2),
		},
		"Cluster-scoped object": {
			obj:       &unstructured.Unstructured{},
			dependson: DependencySet{clusterScopedObj},
			expected:  getDependsOnAnnotation(u1),
		},
		"Multiple objects": {
			obj:       &unstructured.Unstructured{},
			dependson: DependencySet{namespacedObj, clusterScopedObj},
			expected:  getDependsOnAnnotation(multipleAnnotations),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			err := WriteAnnotation(tc.obj, tc.dependson)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error not received")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
				received := getDependsOnAnnotation(tc.obj)
				if received != tc.expected && (received == nil || tc.expected == nil) {
					t.Errorf("\nexpected:\t%#v\nreceived:\t%#v", tc.expected, received)
				}
				if *received != *tc.expected {
					t.Errorf("\nexpected:\t%#v\nreceived:\t%#v", *tc.expected, *received)
				}
			}
		})
	}
}

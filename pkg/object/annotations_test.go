// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package object

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	clusterScopedObj = ObjMetadata{Name: "cluster-obj",
		GroupKind: schema.GroupKind{Group: "test-group", Kind: "test-kind"}}
	namespacedObj = ObjMetadata{Namespace: "test-namespace", Name: "namespaced-obj",
		GroupKind: schema.GroupKind{Group: "test-group", Kind: "test-kind"}}
)

var u1 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "unused",
			"namespace": "unused",
			"annotations": map[string]interface{}{
				DependsOnAnnotation: "test-group/test-kind/cluster-obj",
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
				DependsOnAnnotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
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
				DependsOnAnnotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj, " +
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
				DependsOnAnnotation: "test-group:namespaces:test-namespace:test-kind:namespaced-obj",
			},
		},
	},
}

func TestDependsOnAnnotation(t *testing.T) {
	testCases := map[string]struct {
		obj      *unstructured.Unstructured
		expected []ObjMetadata
		isError  bool
	}{
		"nil object is not found": {
			obj:      nil,
			expected: []ObjMetadata{},
		},
		"Object with no annotations returns not found": {
			obj:      noAnnotations,
			expected: []ObjMetadata{},
		},
		"Unparseable depends on annotation returns not found": {
			obj:      badAnnotation,
			expected: []ObjMetadata{},
			isError:  true,
		},
		"Cluster-scoped object depends on annotation": {
			obj:      u1,
			expected: []ObjMetadata{clusterScopedObj},
		},
		"Namespaced object depends on annotation": {
			obj:      u2,
			expected: []ObjMetadata{namespacedObj},
		},
		"Multiple objects specified in annotation": {
			obj:      multipleAnnotations,
			expected: []ObjMetadata{namespacedObj, clusterScopedObj},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := DependsOnObjs(tc.obj)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error not received")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
				if !SetEquals(tc.expected, actual) {
					t.Errorf("expected (%s), got (%s)", tc.expected, actual)
				}
			}
		})
	}
}

func TestAnnotationToObjMetas(t *testing.T) {
	testCases := map[string]struct {
		annotation string
		expected   []ObjMetadata
		isError    bool
	}{
		"empty annotation is error": {
			annotation: "",
			expected:   []ObjMetadata{},
			isError:    true,
		},
		"wrong number of namespace-scoped fields in annotation is error": {
			annotation: "test-group/test-namespace/test-kind/namespaced-obj",
			expected:   []ObjMetadata{},
			isError:    true,
		},
		"wrong number of cluster-scoped fields in annotation is error": {
			annotation: "test-group/namespaces/test-kind/cluster-obj",
			expected:   []ObjMetadata{},
			isError:    true,
		},
		"cluster-scoped object annotation": {
			annotation: "test-group/test-kind/cluster-obj",
			expected:   []ObjMetadata{clusterScopedObj},
			isError:    false,
		},
		"namespaced object annotation": {
			annotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			expected:   []ObjMetadata{namespacedObj},
			isError:    false,
		},
		"namespaced object annotation with whitespace at ends is valid": {
			annotation: "  test-group/namespaces/test-namespace/test-kind/namespaced-obj\n",
			expected:   []ObjMetadata{namespacedObj},
			isError:    false,
		},
		"multiple object annotation": {
			annotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj," +
				"test-group/test-kind/cluster-obj",
			expected: []ObjMetadata{clusterScopedObj, namespacedObj},
			isError:  false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := DependsOnAnnotationToObjMetas(tc.annotation)
			if err == nil && tc.isError {
				t.Fatalf("expected error, but received none")
			}
			if err != nil && !tc.isError {
				t.Errorf("unexpected error: %s", err)
			}
			if !SetEquals(tc.expected, actual) {
				t.Errorf("expected (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

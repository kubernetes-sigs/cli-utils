// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package dependson

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var (
	clusterScopedObj = object.ObjMetadata{
		Name: "cluster-obj",
		GroupKind: schema.GroupKind{
			Group: "test-group",
			Kind:  "test-kind",
		},
	}
	namespacedObj = object.ObjMetadata{
		Namespace: "test-namespace",
		Name:      "namespaced-obj",
		GroupKind: schema.GroupKind{
			Group: "test-group",
			Kind:  "test-kind",
		},
	}
)

func TestParseDependencySet(t *testing.T) {
	testCases := map[string]struct {
		annotation string
		expected   DependencySet
		isError    bool
	}{
		"empty annotation is error": {
			annotation: "",
			expected:   DependencySet{},
			isError:    true,
		},
		"wrong number of namespace-scoped fields in annotation is error": {
			annotation: "test-group/test-namespace/test-kind/namespaced-obj",
			expected:   DependencySet{},
			isError:    true,
		},
		"wrong number of cluster-scoped fields in annotation is error": {
			annotation: "test-group/namespaces/test-kind/cluster-obj",
			expected:   DependencySet{},
			isError:    true,
		},
		"cluster-scoped object annotation": {
			annotation: "test-group/test-kind/cluster-obj",
			expected:   DependencySet{clusterScopedObj},
			isError:    false,
		},
		"namespaced object annotation": {
			annotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			expected:   DependencySet{namespacedObj},
			isError:    false,
		},
		"namespaced object annotation with whitespace at ends is valid": {
			annotation: "  test-group/namespaces/test-namespace/test-kind/namespaced-obj\n",
			expected:   DependencySet{namespacedObj},
			isError:    false,
		},
		"multiple object annotation": {
			annotation: "test-group/namespaces/test-namespace/test-kind/namespaced-obj," +
				"test-group/test-kind/cluster-obj",
			expected: DependencySet{clusterScopedObj, namespacedObj},
			isError:  false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ParseDependencySet(tc.annotation)
			if err == nil && tc.isError {
				t.Fatalf("expected error, but received none")
			}
			if err != nil && !tc.isError {
				t.Errorf("unexpected error: %s", err)
			}
			if !actual.Equal(tc.expected) {
				t.Errorf("expected (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestParseObjMetadata(t *testing.T) {
	testCases := map[string]struct {
		metaStr  string
		expected object.ObjMetadata
		isError  bool
	}{
		"empty annotation is error": {
			metaStr:  "",
			expected: object.ObjMetadata{},
			isError:  true,
		},
		"wrong number of namespace-scoped fields in annotation is error": {
			metaStr:  "test-group/test-namespace/test-kind/namespaced-obj",
			expected: object.ObjMetadata{},
			isError:  true,
		},
		"wrong number of cluster-scoped fields in annotation is error": {
			metaStr:  "test-group/namespaces/test-kind/cluster-obj",
			expected: object.ObjMetadata{},
			isError:  true,
		},
		"cluster-scoped object annotation": {
			metaStr:  "test-group/test-kind/cluster-obj",
			expected: clusterScopedObj,
			isError:  false,
		},
		"namespaced object annotation": {
			metaStr:  "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			expected: namespacedObj,
			isError:  false,
		},
		"namespaced object annotation with whitespace at ends is valid": {
			metaStr:  "  test-group/namespaces/test-namespace/test-kind/namespaced-obj\n",
			expected: namespacedObj,
			isError:  false,
		},
		"multiple is error": {
			metaStr: "test-group/namespaces/test-namespace/test-kind/namespaced-obj," +
				"test-group/test-kind/cluster-obj",
			expected: object.ObjMetadata{},
			isError:  true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := ParseObjMetadata(tc.metaStr)
			if err == nil && tc.isError {
				t.Fatalf("expected error, but received none")
			}
			if err != nil && !tc.isError {
				t.Errorf("unexpected error: %s", err)
			}
			if actual != tc.expected {
				t.Errorf("expected (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestFormatDependencySet(t *testing.T) {
	testCases := map[string]struct {
		depSet   DependencySet
		expected string
		isError  bool
	}{
		"empty set is not error": {
			depSet:   DependencySet{},
			expected: "",
		},
		"missing kind is error": {
			depSet: DependencySet{
				{
					Name: "cluster-obj",
					GroupKind: schema.GroupKind{
						Group: "test-group",
					},
				},
			},
			expected: "",
			isError:  true,
		},
		"missing name is error": {
			depSet: DependencySet{
				{
					GroupKind: schema.GroupKind{
						Group: "test-group",
						Kind:  "test-kind",
					},
				},
			},
			expected: "",
			isError:  true,
		},
		"cluster-scoped": {
			depSet:   DependencySet{clusterScopedObj},
			expected: "test-group/test-kind/cluster-obj",
			isError:  false,
		},
		"namespace-scoped": {
			depSet:   DependencySet{namespacedObj},
			expected: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			isError:  false,
		},
		"multiple dependencies": {
			depSet: DependencySet{clusterScopedObj, namespacedObj},
			expected: "test-group/test-kind/cluster-obj," +
				"test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			isError: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := FormatDependencySet(tc.depSet)
			if err == nil && tc.isError {
				t.Fatalf("expected error, but received none")
			}
			if err != nil && !tc.isError {
				t.Errorf("unexpected error: %s", err)
			}
			if actual != tc.expected {
				t.Errorf("expected (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestFormatObjMetadata(t *testing.T) {
	testCases := map[string]struct {
		objMeta  object.ObjMetadata
		expected string
		isError  bool
	}{
		"empty is error": {
			objMeta:  object.ObjMetadata{},
			expected: "",
			isError:  true,
		},
		"missing kind is error": {
			objMeta: object.ObjMetadata{
				Name: "cluster-obj",
				GroupKind: schema.GroupKind{
					Group: "test-group",
				},
			},
			expected: "",
			isError:  true,
		},
		"missing name is error": {
			objMeta: object.ObjMetadata{
				GroupKind: schema.GroupKind{
					Group: "test-group",
					Kind:  "test-kind",
				},
			},
			expected: "",
			isError:  true,
		},
		"cluster-scoped": {
			objMeta:  clusterScopedObj,
			expected: "test-group/test-kind/cluster-obj",
			isError:  false,
		},
		"namespace-scoped": {
			objMeta:  namespacedObj,
			expected: "test-group/namespaces/test-namespace/test-kind/namespaced-obj",
			isError:  false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			actual, err := FormatObjMetadata(tc.objMeta)
			if err == nil && tc.isError {
				t.Fatalf("expected error, but received none")
			}
			if err != nil && !tc.isError {
				t.Errorf("unexpected error: %s", err)
			}
			if actual != tc.expected {
				t.Errorf("expected (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

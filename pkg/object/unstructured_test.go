// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var rbac = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]interface{}{
			"name": "test-cluster-role",
		},
	},
}

var testCRD = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata": map[string]interface{}{
			"name": "test-crd",
		},
		"spec": map[string]interface{}{
			"group": "example.com",
			"names": map[string]interface{}{
				"kind": "crontab",
			},
		},
	},
}

var testNamespace = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": "test-namespace",
		},
	},
}

var testPod = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "test-pod",
			"namespace": "test-namespace",
		},
	},
}

var defaultNamespacePod = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "test-pod",
			"namespace": "default",
		},
	},
}

func TestUnstructuredToObjMeta(t *testing.T) {
	tests := map[string]struct {
		obj      *unstructured.Unstructured
		expected ObjMetadata
	}{
		"test RBAC translation": {
			obj: rbac,
			expected: ObjMetadata{
				Name: "test-cluster-role",
				GroupKind: schema.GroupKind{
					Group: "rbac.authorization.k8s.io",
					Kind:  "ClusterRole",
				},
			},
		},
		"test CRD translation": {
			obj: testCRD,
			expected: ObjMetadata{
				Name: "test-crd",
				GroupKind: schema.GroupKind{
					Group: "apiextensions.k8s.io",
					Kind:  "CustomResourceDefinition",
				},
			},
		},
		"test pod translation": {
			obj: testPod,
			expected: ObjMetadata{
				Name:      "test-pod",
				Namespace: "test-namespace",
				GroupKind: schema.GroupKind{
					Group: "",
					Kind:  "Pod",
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := UnstructuredToObjMeta(tc.obj)
			if tc.expected != actual {
				t.Errorf("expected ObjMetadata (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestIsKindNamespace(t *testing.T) {
	tests := map[string]struct {
		obj             *unstructured.Unstructured
		isKindNamespace bool
	}{
		"cluster-scoped RBAC is not a namespace": {
			obj:             rbac,
			isKindNamespace: false,
		},
		"test namespace is a namespace": {
			obj:             testNamespace,
			isKindNamespace: true,
		},
		"test pod is not a namespace": {
			obj:             testPod,
			isKindNamespace: false,
		},
		"default namespaced pod is not a namespace": {
			obj:             defaultNamespacePod,
			isKindNamespace: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := IsKindNamespace(tc.obj)
			if tc.isKindNamespace != actual {
				t.Errorf("expected IsKindNamespace (%t), got (%t) for (%s)",
					tc.isKindNamespace, actual, tc.obj)
			}
		})
	}
}

func TestIsCRD(t *testing.T) {
	tests := map[string]struct {
		obj   *unstructured.Unstructured
		isCRD bool
	}{
		"RBAC is not a CRD": {
			obj:   rbac,
			isCRD: false,
		},
		"test namespace is not a CRD": {
			obj:   testNamespace,
			isCRD: false,
		},
		"test CRD is a CRD": {
			obj:   testCRD,
			isCRD: true,
		},
		"test pod is not a CRD": {
			obj:   testPod,
			isCRD: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := IsCRD(tc.obj)
			if tc.isCRD != actual {
				t.Errorf("expected IsCRD (%t), got (%t) for (%s)", tc.isCRD, actual, tc.obj)
			}
		})
	}
}

func TestIsNamespaced(t *testing.T) {
	tests := map[string]struct {
		obj          *unstructured.Unstructured
		isNamespaced bool
	}{
		"cluster-scoped RBAC is not namespaced": {
			obj:          rbac,
			isNamespaced: false,
		},
		"a CRD is cluster-scoped": {
			obj:          testCRD,
			isNamespaced: false,
		},
		"a namespace is cluster-scoped": {
			obj:          testNamespace,
			isNamespaced: false,
		},
		"pod is namespaced": {
			obj:          testPod,
			isNamespaced: true,
		},
		"default namespaced pod is namespaced": {
			obj:          defaultNamespacePod,
			isNamespaced: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := IsNamespaced(tc.obj)
			if tc.isNamespaced != actual {
				t.Errorf("expected namespaced (%t), got (%t) for (%s)",
					tc.isNamespaced, actual, tc.obj)
			}
		})
	}
}

func TestGetCRDGroupKind(t *testing.T) {
	tests := map[string]struct {
		obj       *unstructured.Unstructured
		isCRD     bool
		groupKind string
	}{
		"RBAC is not a CRD": {
			obj:       rbac,
			isCRD:     false,
			groupKind: "",
		},
		"pod is not a CRD": {
			obj:       testPod,
			isCRD:     false,
			groupKind: "",
		},
		"testCRD has example.com/crontab GroupKind": {
			obj:       testCRD,
			isCRD:     true,
			groupKind: "crontab.example.com",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualGroupKind, actualIsCRD := GetCRDGroupKind(tc.obj)
			if tc.isCRD != actualIsCRD {
				t.Errorf("expected IsCRD (%t), got (%t) for (%s)", tc.isCRD, actualIsCRD, tc.obj)
			}
			if tc.groupKind != actualGroupKind.String() {
				t.Errorf("expected CRD GroupKind (%s), got (%s) for (%s)",
					tc.groupKind, actualGroupKind, tc.obj)
			}
		})
	}
}

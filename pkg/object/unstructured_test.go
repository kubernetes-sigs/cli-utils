// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	. "sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var rbac = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: test-cluster-role
`

var testCRD = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test-crd
spec:
  group: example.com
  scope: Cluster
  names:
    kind: crontab
  versions:
  - name: v1
`

var testCRDv2 = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test-crd
spec:
  group: example.com
  scope: Cluster
  names:
    kind: crontab
  versions:
  - name: v2
`

var testCR = `
apiVersion: example.com/v1
kind: crontab
metadata:
  name: test-cr
`

var testNamespace = `
apiVersion: v1,
kind: Namespace
metadata:
  name: test-namespace
`

var testPod = `
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-namespace
`

var defaultNamespacePod = `
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
`

func TestUnstructuredToObjMeta(t *testing.T) {
	tests := map[string]struct {
		obj      *unstructured.Unstructured
		expected ObjMetadata
	}{
		"test RBAC translation": {
			obj: testutil.Unstructured(t, rbac),
			expected: ObjMetadata{
				Name: "test-cluster-role",
				GroupKind: schema.GroupKind{
					Group: "rbac.authorization.k8s.io",
					Kind:  "ClusterRole",
				},
			},
		},
		"test CRD translation": {
			obj: testutil.Unstructured(t, testCRD),
			expected: ObjMetadata{
				Name: "test-crd",
				GroupKind: schema.GroupKind{
					Group: "apiextensions.k8s.io",
					Kind:  "CustomResourceDefinition",
				},
			},
		},
		"test pod translation": {
			obj: testutil.Unstructured(t, testPod),
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
			actual := UnstructuredToObjMetadata(tc.obj)
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
			obj:             testutil.Unstructured(t, rbac),
			isKindNamespace: false,
		},
		"test namespace is a namespace": {
			obj:             testutil.Unstructured(t, testNamespace),
			isKindNamespace: true,
		},
		"test pod is not a namespace": {
			obj:             testutil.Unstructured(t, testPod),
			isKindNamespace: false,
		},
		"default namespaced pod is not a namespace": {
			obj:             testutil.Unstructured(t, defaultNamespacePod),
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
			obj:   testutil.Unstructured(t, rbac),
			isCRD: false,
		},
		"test namespace is not a CRD": {
			obj:   testutil.Unstructured(t, testNamespace),
			isCRD: false,
		},
		"test CRD is a CRD": {
			obj:   testutil.Unstructured(t, testCRD),
			isCRD: true,
		},
		"test pod is not a CRD": {
			obj:   testutil.Unstructured(t, testPod),
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
			obj:          testutil.Unstructured(t, rbac),
			isNamespaced: false,
		},
		"a CRD is cluster-scoped": {
			obj:          testutil.Unstructured(t, testCRD),
			isNamespaced: false,
		},
		"a namespace is cluster-scoped": {
			obj:          testutil.Unstructured(t, testNamespace),
			isNamespaced: false,
		},
		"pod is namespaced": {
			obj:          testutil.Unstructured(t, testPod),
			isNamespaced: true,
		},
		"default namespaced pod is namespaced": {
			obj:          testutil.Unstructured(t, defaultNamespacePod),
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
			obj:       testutil.Unstructured(t, rbac),
			isCRD:     false,
			groupKind: "",
		},
		"pod is not a CRD": {
			obj:       testutil.Unstructured(t, testPod),
			isCRD:     false,
			groupKind: "",
		},
		"testCRD has example.com/crontab GroupKind": {
			obj:       testutil.Unstructured(t, testCRD),
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

func TestLookupResourceScope(t *testing.T) {
	testCases := map[string]struct {
		resource      *unstructured.Unstructured
		crds          []*unstructured.Unstructured
		expectedScope meta.RESTScope
		expectedErr   error
	}{
		"regular resource": {
			resource:      testutil.Unstructured(t, testPod),
			expectedScope: meta.RESTScopeNamespace,
		},
		"CR not found in the RESTMapper or the provided CRDs": {
			resource: testutil.Unstructured(t, testCR),
			expectedErr: &UnknownTypeError{
				GroupVersionKind: schema.GroupVersionKind{
					Group:   "example.com",
					Version: "v1",
					Kind:    "crontab",
				},
			},
		},
		"CR not found in the RESTMapper or the provided CRDs because version is missing": {
			resource: testutil.Unstructured(t, testCR),
			crds: []*unstructured.Unstructured{
				testutil.Unstructured(t, testCRDv2),
			},
			expectedErr: &UnknownTypeError{
				GroupVersionKind: schema.GroupVersionKind{
					Group:   "example.com",
					Version: "v1",
					Kind:    "crontab",
				},
			},
		},
		"CR found in in the provided CRDs": {
			resource: testutil.Unstructured(t, testCR),
			crds: []*unstructured.Unstructured{
				testutil.Unstructured(t, testCRD),
			},
			expectedScope: meta.RESTScopeRoot,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("test-ns")
			defer tf.Cleanup()

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)

			scope, err := LookupResourceScope(tc.resource, tc.crds, mapper)

			if tc.expectedErr != nil {
				require.Equal(t, tc.expectedErr, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedScope, scope)
		})
	}
}

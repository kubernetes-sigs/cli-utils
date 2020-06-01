// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"k8s.io/kubectl/pkg/scheme"
)

func TestSetNamespaces(t *testing.T) {
	// We need the RESTMapper in the testFactory to contain the CRD
	// types, so add them to the scheme here.
	_ = apiextv1.AddToScheme(scheme.Scheme)

	testCases := map[string]struct {
		infos            []*resource.Info
		defaultNamspace  string
		enforceNamespace bool

		expectedNamespaces []string
		expectedErrText    string
	}{
		"resources already have namespace": {
			infos: []*resource.Info{
				toInfo(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}, "default"),
				toInfo(schema.GroupVersionKind{
					Group:   "policy",
					Version: "v1beta1",
					Kind:    "PodDisruptionBudget",
				}, "default"),
			},
			defaultNamspace:  "foo",
			enforceNamespace: false,
			expectedNamespaces: []string{
				"default",
				"default",
			},
		},
		"resources without namespace and mapping in RESTMapper": {
			infos: []*resource.Info{
				toInfo(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}, ""),
			},
			defaultNamspace:    "foo",
			enforceNamespace:   false,
			expectedNamespaces: []string{"foo"},
		},
		"resource with namespace that does match enforced ns": {
			infos: []*resource.Info{
				toInfo(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}, "bar"),
			},
			defaultNamspace:    "bar",
			enforceNamespace:   true,
			expectedNamespaces: []string{"bar"},
		},
		"resource with namespace that doesn't match enforced ns": {
			infos: []*resource.Info{
				toInfo(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}, "foo"),
			},
			defaultNamspace:  "bar",
			enforceNamespace: true,
			expectedErrText:  "does not match the namespace",
		},
		"cluster-scoped CR with CRD": {
			infos: []*resource.Info{
				toInfo(schema.GroupVersionKind{
					Group:   "custom.io",
					Version: "v1",
					Kind:    "Custom",
				}, ""),
				toCRDInfo(schema.GroupVersionKind{
					Group:   "apiextensions.k8s.io",
					Version: "v1",
					Kind:    "CustomResourceDefinition",
				}, schema.GroupKind{
					Group: "custom.io",
					Kind:  "Custom",
				}, "Cluster"),
			},
			defaultNamspace:    "bar",
			enforceNamespace:   true,
			expectedNamespaces: []string{"", ""},
		},
		"namespace-scoped CR with CRD": {
			infos: []*resource.Info{
				toCRDInfo(schema.GroupVersionKind{
					Group:   "apiextensions.k8s.io",
					Version: "v1",
					Kind:    "CustomResourceDefinition",
				}, schema.GroupKind{
					Group: "custom.io",
					Kind:  "Custom",
				}, "Namespaced"),
				toInfo(schema.GroupVersionKind{
					Group:   "custom.io",
					Version: "v1",
					Kind:    "Custom",
				}, ""),
			},
			defaultNamspace:    "bar",
			enforceNamespace:   true,
			expectedNamespaces: []string{"", "bar"},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("namespace")
			defer tf.Cleanup()

			err := setNamespaces(tf, tc.infos, tc.defaultNamspace, tc.enforceNamespace)

			if tc.expectedErrText != "" {
				if err == nil {
					t.Errorf("expected error %s, but not nil", tc.expectedErrText)
				}
				assert.Contains(t, err.Error(), tc.expectedErrText)
				return
			}

			assert.NoError(t, err)

			for i, inf := range tc.infos {
				expectedNs := tc.expectedNamespaces[i]
				assert.Equal(t, expectedNs, inf.Namespace)
				accessor, _ := meta.Accessor(inf.Object)
				assert.Equal(t, expectedNs, accessor.GetNamespace())
			}
		})
	}
}

func toInfo(gvk schema.GroupVersionKind, namespace string) *resource.Info {
	return &resource.Info{
		Namespace: namespace,
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": gvk.GroupVersion().String(),
				"kind":       gvk.Kind,
				"metadata": map[string]interface{}{
					"namespace": namespace,
				},
			},
		},
	}
}

func toCRDInfo(gvk schema.GroupVersionKind, gk schema.GroupKind,
	scope string) *resource.Info {
	return &resource.Info{
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": gvk.GroupVersion().String(),
				"kind":       gvk.Kind,
				"spec": map[string]interface{}{
					"group": gk.Group,
					"names": map[string]interface{}{
						"kind": gk.Kind,
					},
					"scope": scope,
				},
			},
		},
	}
}

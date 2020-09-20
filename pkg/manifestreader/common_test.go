// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
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

			err := SetNamespaces(tf, tc.infos, tc.defaultNamspace, tc.enforceNamespace)

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

var (
	depID = object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Namespace: "default",
		Name:      "foo",
	}

	clusterRoleID = object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "rbac.authorization.k8s.io",
			Kind:  "ClusterRole",
		},
		Name: "bar",
	}
)

func TestFilterLocalConfigs(t *testing.T) {
	testCases := map[string]struct {
		input    []*resource.Info
		expected []string
	}{
		"don't filter if no annotation": {
			input: []*resource.Info{
				objMetaToInfo(depID),
				objMetaToInfo(clusterRoleID),
			},
			expected: []string{
				depID.Name,
				clusterRoleID.Name,
			},
		},
		"filter all if all have annotation": {
			input: []*resource.Info{
				addAnnotation(t, objMetaToInfo(depID), filters.LocalConfigAnnotation, "true"),
				addAnnotation(t, objMetaToInfo(clusterRoleID), filters.LocalConfigAnnotation, "false"),
			},
			expected: []string{},
		},
		"filter even if resource have other annotations": {
			input: []*resource.Info{
				addAnnotation(t,
					addAnnotation(
						t, objMetaToInfo(depID),
						filters.LocalConfigAnnotation, "true"),
					"my-annotation", "foo"),
			},
			expected: []string{},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			res := FilterLocalConfig(tc.input)

			var names []string
			for _, inf := range res {
				names = append(names, inf.Name)
			}

			// Avoid test failures due to nil slice and empty slice
			// not being equal.
			if len(tc.expected) == 0 && len(names) == 0 {
				return
			}
			assert.Equal(t, tc.expected, names)
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

func objMetaToInfo(id object.ObjMetadata) *resource.Info {
	return &resource.Info{
		Namespace: id.Namespace,
		Name:      id.Name,
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": fmt.Sprintf("%s/v1", id.GroupKind.Group),
				"kind":       id.GroupKind.Kind,
				"metadata": map[string]interface{}{
					"namespace": id.Namespace,
					"name":      id.Name,
				},
			},
		},
	}
}

func addAnnotation(t *testing.T, info *resource.Info, name, val string) *resource.Info {
	u := info.Object.(*unstructured.Unstructured)
	annos, found, err := unstructured.NestedStringMap(u.Object, "metadata", "annotations")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		annos = make(map[string]string)
	}
	annos[name] = val
	err = unstructured.SetNestedStringMap(u.Object, annos, "metadata", "annotations")
	if err != nil {
		t.Fatal(err)
	}
	return info
}

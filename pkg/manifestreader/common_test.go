// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestSetNamespaces(t *testing.T) {
	testCases := map[string]struct {
		objs             []*unstructured.Unstructured
		defaultNamspace  string
		enforceNamespace bool

		expectedNamespaces []string
		expectedErr        error
	}{
		"resources already have namespace": {
			objs: []*unstructured.Unstructured{
				toUnstructured(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}, "default"),
				toUnstructured(schema.GroupVersionKind{
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
			objs: []*unstructured.Unstructured{
				toUnstructured(schema.GroupVersionKind{
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
			objs: []*unstructured.Unstructured{
				toUnstructured(schema.GroupVersionKind{
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
			objs: []*unstructured.Unstructured{
				toUnstructured(schema.GroupVersionKind{
					Group:   "apps",
					Version: "v1",
					Kind:    "Deployment",
				}, "foo"),
			},
			defaultNamspace:  "bar",
			enforceNamespace: true,
			expectedErr: &NamespaceMismatchError{
				RequiredNamespace: "bar",
				Namespace:         "foo",
			},
		},
		"cluster-scoped CR with CRD": {
			objs: []*unstructured.Unstructured{
				toUnstructured(schema.GroupVersionKind{
					Group:   "custom.io",
					Version: "v1",
					Kind:    "Custom",
				}, ""),
				toCRDUnstructured(schema.GroupVersionKind{
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
			objs: []*unstructured.Unstructured{
				toCRDUnstructured(schema.GroupVersionKind{
					Group:   "apiextensions.k8s.io",
					Version: "v1",
					Kind:    "CustomResourceDefinition",
				}, schema.GroupKind{
					Group: "custom.io",
					Kind:  "Custom",
				}, "Namespaced"),
				toUnstructured(schema.GroupVersionKind{
					Group:   "custom.io",
					Version: "v1",
					Kind:    "Custom",
				}, ""),
			},
			defaultNamspace:    "bar",
			enforceNamespace:   true,
			expectedNamespaces: []string{"", "bar"},
		},
		"unknown types in CRs": {
			objs: []*unstructured.Unstructured{
				toUnstructured(schema.GroupVersionKind{
					Group:   "custom.io",
					Version: "v1",
					Kind:    "Custom",
				}, ""),
				toUnstructured(schema.GroupVersionKind{
					Group:   "custom.io",
					Version: "v1",
					Kind:    "AnotherCustom",
				}, ""),
			},
			expectedErr: &UnknownTypesError{
				GroupKinds: []schema.GroupKind{
					{
						Group: "custom.io",
						Kind:  "Custom",
					},
					{
						Group: "custom.io",
						Kind:  "AnotherCustom",
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("namespace")
			defer tf.Cleanup()

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)
			crdGV := schema.GroupVersion{Group: "apiextensions.k8s.io", Version: "v1"}
			crdMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{crdGV})
			crdMapper.AddSpecific(crdGV.WithKind("CustomResourceDefinition"),
				crdGV.WithResource("customresourcedefinitions"),
				crdGV.WithResource("customresourcedefinition"), meta.RESTScopeRoot)
			mapper = meta.MultiRESTMapper([]meta.RESTMapper{mapper, crdMapper})

			err = SetNamespaces(mapper, tc.objs, tc.defaultNamspace, tc.enforceNamespace)

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedErr, err)
				return
			}

			assert.NoError(t, err)

			for i, obj := range tc.objs {
				assert.Equal(t, tc.expectedNamespaces[i], obj.GetNamespace())
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
		input    []*unstructured.Unstructured
		expected []string
	}{
		"don't filter if no annotation": {
			input: []*unstructured.Unstructured{
				objMetaToUnstructured(depID),
				objMetaToUnstructured(clusterRoleID),
			},
			expected: []string{
				depID.Name,
				clusterRoleID.Name,
			},
		},
		"filter all if all have annotation": {
			input: []*unstructured.Unstructured{
				addAnnotation(t, objMetaToUnstructured(depID), filters.LocalConfigAnnotation, "true"),
				addAnnotation(t, objMetaToUnstructured(clusterRoleID), filters.LocalConfigAnnotation, "false"),
			},
			expected: []string{},
		},
		"filter even if resource have other annotations": {
			input: []*unstructured.Unstructured{
				addAnnotation(t,
					addAnnotation(
						t, objMetaToUnstructured(depID),
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
			for _, obj := range res {
				names = append(names, obj.GetName())
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

func TestRemoveAnnotations(t *testing.T) {
	testCases := map[string]struct {
		node                *yaml.RNode
		removeAnnotations   []kioutil.AnnotationKey
		expectedAnnotations []kioutil.AnnotationKey
	}{
		"filter both kioutil annotations": {
			node: yaml.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo
  annotations:
    config.kubernetes.io/path: deployment.yaml
    config.kubernetes.io/index: 0
`),
			removeAnnotations: []kioutil.AnnotationKey{
				kioutil.PathAnnotation,
				kioutil.IndexAnnotation,
			},
		},
		"filter only a subset of the annotations": {
			node: yaml.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo
  annotations:
    internal.config.kubernetes.io/path: deployment.yaml
    internal.config.kubernetes.io/index: 0
`),
			removeAnnotations: []kioutil.AnnotationKey{
				kioutil.IndexAnnotation,
			},
			expectedAnnotations: []kioutil.AnnotationKey{
				kioutil.PathAnnotation,
			},
		},
		"filter none of the annotations": {
			node: yaml.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo
  annotations:
    internal.config.kubernetes.io/path: deployment.yaml
    internal.config.kubernetes.io/index: 0
`),
			removeAnnotations: []kioutil.AnnotationKey{},
			expectedAnnotations: []kioutil.AnnotationKey{
				kioutil.PathAnnotation,
				kioutil.IndexAnnotation,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			node := tc.node
			err := RemoveAnnotations(node, tc.removeAnnotations...)
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			for _, anno := range tc.removeAnnotations {
				n, err := node.Pipe(yaml.GetAnnotation(anno))
				if !assert.NoError(t, err) {
					t.FailNow()
				}
				assert.Nil(t, n)
			}
			for _, anno := range tc.expectedAnnotations {
				n, err := node.Pipe(yaml.GetAnnotation(anno))
				if !assert.NoError(t, err) {
					t.FailNow()
				}
				assert.NotNil(t, n)
			}
		})
	}
}

func toUnstructured(gvk schema.GroupVersionKind, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvk.GroupVersion().String(),
			"kind":       gvk.Kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
			},
		},
	}
}

func toCRDUnstructured(gvk schema.GroupVersionKind, gk schema.GroupKind,
	scope string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
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
	}
}

func objMetaToUnstructured(id object.ObjMetadata) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/v1", id.GroupKind.Group),
			"kind":       id.GroupKind.Kind,
			"metadata": map[string]interface{}{
				"namespace": id.Namespace,
				"name":      id.Name,
			},
		},
	}
}

func addAnnotation(t *testing.T, u *unstructured.Unstructured, name, val string) *unstructured.Unstructured {
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
	return u
}

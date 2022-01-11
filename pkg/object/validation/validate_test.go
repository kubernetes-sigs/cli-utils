// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/multierror"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestValidate(t *testing.T) {
	testCases := map[string]struct {
		resources     []*unstructured.Unstructured
		expectedError error
	}{
		"missing kind": {
			resources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"metadata": map[string]interface{}{
							"name":      "foo",
							"namespace": "default",
						},
					},
				},
			},
			expectedError: validation.NewError(
				&field.Error{
					Type:     field.ErrorTypeRequired,
					Field:    "kind",
					BadValue: "",
					Detail:   "kind is required",
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "",
					},
					Name:      "foo",
					Namespace: "default",
				},
			),
		},
		"multiple errors in one object": {
			resources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
					},
				},
			},
			expectedError: validation.NewError(
				multierror.New(
					&field.Error{
						Type:     field.ErrorTypeRequired,
						Field:    "metadata.name",
						BadValue: "",
						Detail:   "name is required",
					},
					&field.Error{
						Type:     field.ErrorTypeRequired,
						Field:    "metadata.namespace",
						BadValue: "",
						Detail:   "namespace is required",
					},
				),
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Name:      "",
					Namespace: "",
				},
			),
		},
		"one error in multiple object": {
			resources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"namespace": "default",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "StatefulSet",
						"metadata": map[string]interface{}{
							"namespace": "default",
						},
					},
				},
			},
			expectedError: multierror.New(
				validation.NewError(
					&field.Error{
						Type:     field.ErrorTypeRequired,
						Field:    "metadata.name",
						BadValue: "",
						Detail:   "name is required",
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Name:      "",
						Namespace: "default",
					},
				),
				validation.NewError(
					&field.Error{
						Type:     field.ErrorTypeRequired,
						Field:    "metadata.name",
						BadValue: "",
						Detail:   "name is required",
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "StatefulSet",
						},
						Name:      "",
						Namespace: "default",
					},
				),
			),
		},
		"namespace must be empty (cluster-scoped)": {
			resources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"metadata": map[string]interface{}{
							"name":      "foo",
							"namespace": "default",
						},
					},
				},
			},
			expectedError: validation.NewError(
				&field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "metadata.namespace",
					BadValue: "default",
					Detail:   "namespace must be empty",
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Namespace",
					},
					Name:      "foo",
					Namespace: "default",
				},
			),
		},
		"namespace is required (namespace-scoped)": {
			resources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name": "foo",
						},
					},
				},
			},
			expectedError: validation.NewError(
				&field.Error{
					Type:     field.ErrorTypeRequired,
					Field:    "metadata.namespace",
					BadValue: "",
					Detail:   "namespace is required",
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Name:      "foo",
					Namespace: "",
				},
			),
		},
		"scope for CRs are found in CRDs if available": {
			resources: []*unstructured.Unstructured{
				testutil.Unstructured(t, `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: customs.custom.io
spec:
  group: custom.io
  names:
    kind: Custom
  scope: Cluster
  versions:
  - name: v1
`,
				),
				testutil.Unstructured(t, `
apiVersion: custom.io/v1
kind: Custom
metadata:
  name: foo
  namespace: default
`,
				),
			},
			expectedError: validation.NewError(
				&field.Error{
					Type:     field.ErrorTypeInvalid,
					Field:    "metadata.namespace",
					BadValue: "default",
					Detail:   "namespace must be empty",
				},
				object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "custom.io",
						Kind:  "Custom",
					},
					Name:      "foo",
					Namespace: "default",
				},
			),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("test-ns")
			defer tf.Cleanup()

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)
			crdGV := schema.GroupVersion{Group: "apiextensions.k8s.io", Version: "v1"}
			crdMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{crdGV})
			crdMapper.AddSpecific(crdGV.WithKind("CustomResourceDefinition"),
				crdGV.WithResource("customresourcedefinitions"),
				crdGV.WithResource("customresourcedefinition"), meta.RESTScopeRoot)
			mapper = meta.MultiRESTMapper([]meta.RESTMapper{mapper, crdMapper})

			validator := &validation.Validator{
				Mapper: mapper,
			}
			err = validator.Validate(tc.resources)
			if tc.expectedError == nil {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.expectedError.Error())
		})
	}
}

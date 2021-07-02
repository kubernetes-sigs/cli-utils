// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestValidate(t *testing.T) {
	_ = apiextv1.AddToScheme(scheme.Scheme)
	testCases := map[string]struct {
		resources     []*unstructured.Unstructured
		expectedError error
	}{
		"errors are reported for resources": {
			resources: []*unstructured.Unstructured{
				testutil.Unstructured(t, `
apiVersion: apps/v1
kind: Deployment
`,
				),
			},
			expectedError: &object.MultiValidationError{
				Errors: []*object.ValidationError{
					{
						GroupVersionKind: schema.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Name:      "",
						Namespace: "",
						FieldErrors: []*field.Error{
							{
								Type:     field.ErrorTypeRequired,
								Field:    "metadata.name",
								BadValue: "",
								Detail:   "name is required",
							},
							{
								Type:     field.ErrorTypeRequired,
								Field:    "metadata.namespace",
								BadValue: "",
								Detail:   "namespace is required",
							},
						},
					},
				},
			},
		},
		"error is reported for all resources": {
			resources: []*unstructured.Unstructured{
				testutil.Unstructured(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: default
`),
				testutil.Unstructured(t, `
apiVersion: apps/v1
kind: StatefulSet
metadata:
  namespace: default
`,
				),
			},
			expectedError: &object.MultiValidationError{
				Errors: []*object.ValidationError{
					{
						GroupVersionKind: schema.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Name:      "",
						Namespace: "default",
						FieldErrors: []*field.Error{
							{
								Type:     field.ErrorTypeRequired,
								Field:    "metadata.name",
								BadValue: "",
								Detail:   "name is required",
							},
						},
					},
					{
						GroupVersionKind: schema.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "StatefulSet",
						},
						Name:      "",
						Namespace: "default",
						FieldErrors: []*field.Error{
							{
								Type:     field.ErrorTypeRequired,
								Field:    "metadata.name",
								BadValue: "",
								Detail:   "name is required",
							},
						},
					},
				},
			},
		},
		"error is reported if a cluster-scoped resource has namespace set": {
			resources: []*unstructured.Unstructured{
				testutil.Unstructured(t, `
apiVersion: v1
kind: Namespace
metadata:
  name: foo
  namespace: default
`,
				),
			},
			expectedError: &object.MultiValidationError{
				Errors: []*object.ValidationError{
					{
						GroupVersionKind: schema.GroupVersionKind{
							Group:   "",
							Version: "v1",
							Kind:    "Namespace",
						},
						Name:      "foo",
						Namespace: "default",
						FieldErrors: []*field.Error{
							{
								Type:     field.ErrorTypeInvalid,
								Field:    "metadata.namespace",
								BadValue: "default",
								Detail:   "namespace must be empty",
							},
						},
					},
				},
			},
		},
		"error is reported if a namespace-scoped resource doesn't have namespace set": {
			resources: []*unstructured.Unstructured{
				testutil.Unstructured(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo
`,
				),
			},
			expectedError: &object.MultiValidationError{
				Errors: []*object.ValidationError{
					{
						GroupVersionKind: schema.GroupVersionKind{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Name:      "foo",
						Namespace: "",
						FieldErrors: []*field.Error{
							{
								Type:     field.ErrorTypeRequired,
								Field:    "metadata.namespace",
								BadValue: "",
								Detail:   "namespace is required",
							},
						},
					},
				},
			},
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
			expectedError: &object.MultiValidationError{
				Errors: []*object.ValidationError{
					{
						GroupVersionKind: schema.GroupVersionKind{
							Group:   "custom.io",
							Version: "v1",
							Kind:    "Custom",
						},
						Name:      "foo",
						Namespace: "default",
						FieldErrors: []*field.Error{
							{
								Type:     field.ErrorTypeInvalid,
								Field:    "metadata.namespace",
								BadValue: "default",
								Detail:   "namespace must be empty",
							},
						},
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("test-ns")
			defer tf.Cleanup()

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)

			err = (&object.Validator{
				Mapper: mapper,
			}).Validate(tc.resources)

			if tc.expectedError == nil {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}

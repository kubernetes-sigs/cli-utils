// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
)

func TestUnstructuredToInfo(t *testing.T) {
	testCases := map[string]struct {
		obj               *unstructured.Unstructured
		expectedSource    string
		expectedName      string
		expectedNamespace string
	}{
		"with path annotation": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name": "foo",
						"annotations": map[string]interface{}{
							kioutil.PathAnnotation: "deployment.yaml",
						},
					},
				},
			},
			expectedSource:    "deployment.yaml",
			expectedName:      "foo",
			expectedNamespace: "",
		},
		"with both new and legacy annotations": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name": "foo",
						"annotations": map[string]interface{}{
							kioutil.PathAnnotation:       "deployment.yaml",
							kioutil.LegacyPathAnnotation: "deployment.yaml", //nolint:staticcheck
						},
					},
				},
			},
			expectedSource:    "deployment.yaml",
			expectedName:      "foo",
			expectedNamespace: "",
		},
		"without path annotation": {
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
				},
			},
			expectedSource:    "unstructured",
			expectedName:      "foo",
			expectedNamespace: "bar",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			inf, err := UnstructuredToInfo(tc.obj)
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			assert.Equal(t, tc.expectedSource, inf.Source)
			assert.Equal(t, tc.expectedName, inf.Name)
			assert.Equal(t, tc.expectedNamespace, inf.Namespace)

			u := inf.Object.(*unstructured.Unstructured)
			annos, found, err := unstructured.NestedStringMap(u.Object, "metadata", "annotations")
			if !assert.NoError(t, err) {
				t.FailNow()
			}

			if found {
				for _, a := range []kioutil.AnnotationKey{kioutil.PathAnnotation, kioutil.LegacyPathAnnotation} { //nolint:staticcheck
					_, hasAnnotation := annos[a]
					assert.False(t, hasAnnotation, "did not expect %s", a)
				}
			}
		})
	}
}

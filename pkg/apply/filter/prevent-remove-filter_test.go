// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var defaultObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "pod-name",
			"namespace": "test-namespace",
		},
	},
}

func TestPreventDeleteAnnotation(t *testing.T) {
	tests := map[string]struct {
		annotations   map[string]string
		expectedError error
	}{
		"Nil map returns false": {
			annotations: nil,
		},
		"Empty map returns false": {
			annotations: map[string]string{},
		},
		"Wrong annotation key/value is false": {
			annotations: map[string]string{
				"foo": "bar",
			},
		},
		"Annotation key without value is false": {
			annotations: map[string]string{
				common.OnRemoveAnnotation: "bar",
			},
		},
		"Annotation key and value is true": {
			annotations: map[string]string{
				common.OnRemoveAnnotation: common.OnRemoveKeep,
			},
			expectedError: &AnnotationPreventedDeletionError{
				Annotation: common.OnRemoveAnnotation,
				Value:      common.OnRemoveKeep,
			},
		},
		"Annotation key client.lifecycle.config.k8s.io/deletion without value is false": {
			annotations: map[string]string{
				common.LifecycleDeleteAnnotation: "any",
			},
		},
		"Annotation key client.lifecycle.config.k8s.io/deletion and value is true": {
			annotations: map[string]string{
				common.LifecycleDeleteAnnotation: common.PreventDeletion,
			},
			expectedError: &AnnotationPreventedDeletionError{
				Annotation: common.LifecycleDeleteAnnotation,
				Value:      common.PreventDeletion,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := PreventRemoveFilter{}
			obj := defaultObj.DeepCopy()
			obj.SetAnnotations(tc.annotations)
			err := filter.Filter(obj)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/common"
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
		annotations map[string]string
		expected    bool
	}{
		"Nil map returns false": {
			annotations: nil,
			expected:    false,
		},
		"Empty map returns false": {
			annotations: map[string]string{},
			expected:    false,
		},
		"Wrong annotation key/value is false": {
			annotations: map[string]string{
				"foo": "bar",
			},
			expected: false,
		},
		"Annotation key without value is false": {
			annotations: map[string]string{
				common.OnRemoveAnnotation: "bar",
			},
			expected: false,
		},
		"Annotation key and value is true": {
			annotations: map[string]string{
				common.OnRemoveAnnotation: common.OnRemoveKeep,
			},
			expected: true,
		},
		"Annotation key client.lifecycle.config.k8s.io/deletion without value is false": {
			annotations: map[string]string{
				common.LifecycleDeleteAnnotation: "any",
			},
			expected: false,
		},
		"Annotation key client.lifecycle.config.k8s.io/deletion and value is true": {
			annotations: map[string]string{
				common.LifecycleDeleteAnnotation: common.PreventDeletion,
			},
			expected: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := PreventRemoveFilter{}
			obj := defaultObj.DeepCopy()
			obj.SetAnnotations(tc.annotations)
			ctx := context.TODO()
			actual, reason, err := filter.Filter(ctx, obj)
			if err != nil {
				t.Fatalf("PreventRemoveFilter unexpected error (%s)", err)
			}
			if tc.expected != actual {
				t.Errorf("PreventRemoveFilter expected (%t), got (%t)", tc.expected, actual)
			}
			if tc.expected && len(reason) == 0 {
				t.Errorf("PreventRemoveFilter expected Reason, but none found")
			}
			if !tc.expected && len(reason) > 0 {
				t.Errorf("PreventRemoveFilter expected no Reason, but found (%s)", reason)
			}
		})
	}
}

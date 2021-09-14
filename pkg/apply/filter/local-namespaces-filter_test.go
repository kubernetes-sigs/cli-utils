// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
)

var testNamespace = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": "test-namespace",
		},
	},
}

func TestLocalNamespacesFilter(t *testing.T) {
	tests := map[string]struct {
		localNamespaces sets.String
		namespace       string
		filtered        bool
	}{
		"No local namespaces, namespace is not filtered": {
			localNamespaces: sets.NewString(),
			namespace:       "test-namespace",
			filtered:        false,
		},
		"Namespace not in local namespaces, namespace is not filtered": {
			localNamespaces: sets.NewString("foo", "bar"),
			namespace:       "test-namespace",
			filtered:        false,
		},
		"Namespace is in local namespaces, namespace is filtered": {
			localNamespaces: sets.NewString("foo", "test-namespace", "bar"),
			namespace:       "test-namespace",
			filtered:        true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := LocalNamespacesFilter{
				LocalNamespaces: tc.localNamespaces,
			}
			namespace := testNamespace.DeepCopy()
			namespace.SetName(tc.namespace)
			ctx := context.TODO()
			actual, reason, err := filter.Filter(ctx, namespace)
			if err != nil {
				t.Fatalf("LocalNamespacesFilter unexpected error (%s)", err)
			}
			if tc.filtered != actual {
				t.Errorf("LocalNamespacesFilter expected filter (%t), got (%t)", tc.filtered, actual)
			}
			if tc.filtered && len(reason) == 0 {
				t.Errorf("LocalNamespacesFilter filtered; expected but missing Reason")
			}
			if !tc.filtered && len(reason) > 0 {
				t.Errorf("LocalNamespacesFilter not filtered; received unexpected Reason: %s", reason)
			}
		})
	}
}

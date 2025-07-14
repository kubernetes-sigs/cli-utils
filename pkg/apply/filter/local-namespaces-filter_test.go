// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var testNamespace = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": "test-namespace",
		},
	},
}

func TestLocalNamespacesFilter(t *testing.T) {
	tests := map[string]struct {
		localNamespaces sets.String // nolint:staticcheck
		namespace       string
		expectedError   error
	}{
		"No local namespaces, namespace is not filtered": {
			localNamespaces: sets.NewString(),
			namespace:       "test-namespace",
		},
		"Namespace not in local namespaces, namespace is not filtered": {
			localNamespaces: sets.NewString("foo", "bar"),
			namespace:       "test-namespace",
		},
		"Namespace is in local namespaces, namespace is filtered": {
			localNamespaces: sets.NewString("foo", "test-namespace", "bar"),
			namespace:       "test-namespace",
			expectedError: &NamespaceInUseError{
				Namespace: "test-namespace",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := LocalNamespacesFilter{
				LocalNamespaces: tc.localNamespaces,
			}
			obj := testNamespace.DeepCopy()
			obj.SetName(tc.namespace)
			err := filter.Filter(t.Context(), obj)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestEdgeSort(t *testing.T) {
	testCases := map[string]struct {
		edges    []Edge
		expected []Edge
	}{
		"one edge": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
			},
		},
		"two edges no change": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
			},
		},
		"two edges same from": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
			},
		},
		"two edges": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
			},
		},
		"two edges by name": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj3", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
				{
					From: object.ObjMetadata{Name: "obj1", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
				{
					From: object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj3", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
			},
		},
		"three edges": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj3"},
					To:   object.ObjMetadata{Name: "obj4"},
				},
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
				{
					From: object.ObjMetadata{Name: "obj3"},
					To:   object.ObjMetadata{Name: "obj4"},
				},
			},
		},
		"two edges cycle": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj1", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
				{
					From: object.ObjMetadata{Name: "obj1", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
				{
					From: object.ObjMetadata{Name: "obj2", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
					To:   object.ObjMetadata{Name: "obj1", Namespace: "ns1", GroupKind: schema.GroupKind{Kind: "Pod"}},
				},
			},
		},
		"three edges cycle": {
			edges: []Edge{
				{
					From: object.ObjMetadata{Name: "obj3"},
					To:   object.ObjMetadata{Name: "obj1"},
				},
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
			},
			expected: []Edge{
				{
					From: object.ObjMetadata{Name: "obj1"},
					To:   object.ObjMetadata{Name: "obj2"},
				},
				{
					From: object.ObjMetadata{Name: "obj2"},
					To:   object.ObjMetadata{Name: "obj3"},
				},
				{
					From: object.ObjMetadata{Name: "obj3"},
					To:   object.ObjMetadata{Name: "obj1"},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			sort.Sort(SortableEdges(tc.edges))
			assert.Equal(t, tc.expected, tc.edges)
		})
	}
}

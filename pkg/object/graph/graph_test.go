// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// This package provides a graph data struture
// and graph functionality.
package graph

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var (
	o1 = object.ObjMetadata{Name: "obj1", GroupKind: schema.GroupKind{Group: "test", Kind: "foo"}}
	o2 = object.ObjMetadata{Name: "obj2", GroupKind: schema.GroupKind{Group: "test", Kind: "foo"}}
	o3 = object.ObjMetadata{Name: "obj3", GroupKind: schema.GroupKind{Group: "test", Kind: "foo"}}
	o4 = object.ObjMetadata{Name: "obj4", GroupKind: schema.GroupKind{Group: "test", Kind: "foo"}}
	o5 = object.ObjMetadata{Name: "obj5", GroupKind: schema.GroupKind{Group: "test", Kind: "foo"}}
)

var (
	e1 Edge = Edge{From: o1, To: o2}
	e2 Edge = Edge{From: o2, To: o3}
	e3 Edge = Edge{From: o1, To: o3}
	e4 Edge = Edge{From: o3, To: o4}
	e5 Edge = Edge{From: o2, To: o4}
	e6 Edge = Edge{From: o2, To: o1}
	e7 Edge = Edge{From: o3, To: o1}
	e8 Edge = Edge{From: o4, To: o5}
)

func TestObjectGraphSort(t *testing.T) {
	testCases := map[string]struct {
		vertices []object.ObjMetadata
		edges    []Edge
		expected [][]object.ObjMetadata
		isError  bool
	}{
		"one edge": {
			vertices: []object.ObjMetadata{o1, o2},
			edges:    []Edge{e1},
			expected: [][]object.ObjMetadata{{o2}, {o1}},
			isError:  false,
		},
		"two edges": {
			vertices: []object.ObjMetadata{o1, o2, o3},
			edges:    []Edge{e1, e2},
			expected: [][]object.ObjMetadata{{o3}, {o2}, {o1}},
			isError:  false,
		},
		"three edges": {
			vertices: []object.ObjMetadata{o1, o2, o3},
			edges:    []Edge{e1, e3, e2},
			expected: [][]object.ObjMetadata{{o3}, {o2}, {o1}},
			isError:  false,
		},
		"four edges": {
			vertices: []object.ObjMetadata{o1, o2, o3, o4},
			edges:    []Edge{e1, e2, e4, e5},
			expected: [][]object.ObjMetadata{{o4}, {o3}, {o2}, {o1}},
			isError:  false,
		},
		"five edges": {
			vertices: []object.ObjMetadata{o1, o2, o3, o4},
			edges:    []Edge{e5, e1, e3, e2, e4},
			expected: [][]object.ObjMetadata{{o4}, {o3}, {o2}, {o1}},
			isError:  false,
		},
		"no edges means all in the same first set": {
			vertices: []object.ObjMetadata{o1, o2, o3, o4},
			edges:    []Edge{},
			expected: [][]object.ObjMetadata{{o4, o3, o2, o1}},
			isError:  false,
		},
		"multiple objects in first set": {
			vertices: []object.ObjMetadata{o1, o2, o3, o4, o5},
			edges:    []Edge{e1, e2, e5, e8},
			expected: [][]object.ObjMetadata{{o5, o3}, {o4}, {o2}, {o1}},
			isError:  false,
		},
		"simple cycle in graph is an error": {
			vertices: []object.ObjMetadata{o1, o2},
			edges:    []Edge{e1, e6},
			expected: [][]object.ObjMetadata{},
			isError:  true,
		},
		"multi-edge cycle in graph is an error": {
			vertices: []object.ObjMetadata{o1, o2, o3},
			edges:    []Edge{e1, e2, e7},
			expected: [][]object.ObjMetadata{},
			isError:  true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g := New()
			for _, vertex := range tc.vertices {
				g.AddVertex(vertex)
			}
			for _, edge := range tc.edges {
				g.AddEdge(edge.From, edge.To)
			}
			actual, err := g.Sort()
			if err == nil && tc.isError {
				t.Fatalf("expected error, but received none")
			}
			if err != nil && !tc.isError {
				t.Errorf("unexpected error: %s", err)
			}
			if !tc.isError {
				if len(actual) != len(tc.expected) {
					t.Errorf("expected (%s), got (%s)", tc.expected, actual)
				}
				for i, actualSet := range actual {
					expectedSet := tc.expected[i]
					if !object.SetEquals(expectedSet, actualSet) {
						t.Errorf("expected sorted objects (%s), got (%s)", tc.expected, actual)
					}
				}
			}
		})
	}
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// This package provides a graph data struture
// and graph functionality.
package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
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
		vertices      object.ObjMetadataSet
		edges         []Edge
		expected      []object.ObjMetadataSet
		expectedError error
	}{
		"one edge": {
			vertices: object.ObjMetadataSet{o1, o2},
			edges:    []Edge{e1},
			expected: []object.ObjMetadataSet{{o2}, {o1}},
		},
		"two edges": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges:    []Edge{e1, e2},
			expected: []object.ObjMetadataSet{{o3}, {o2}, {o1}},
		},
		"three edges": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges:    []Edge{e1, e3, e2},
			expected: []object.ObjMetadataSet{{o3}, {o2}, {o1}},
		},
		"four edges": {
			vertices: object.ObjMetadataSet{o1, o2, o3, o4},
			edges:    []Edge{e1, e2, e4, e5},
			expected: []object.ObjMetadataSet{{o4}, {o3}, {o2}, {o1}},
		},
		"five edges": {
			vertices: object.ObjMetadataSet{o1, o2, o3, o4},
			edges:    []Edge{e5, e1, e3, e2, e4},
			expected: []object.ObjMetadataSet{{o4}, {o3}, {o2}, {o1}},
		},
		"no edges means all in the same first set": {
			vertices: object.ObjMetadataSet{o1, o2, o3, o4},
			edges:    []Edge{},
			expected: []object.ObjMetadataSet{{o4, o3, o2, o1}},
		},
		"multiple objects in first set": {
			vertices: object.ObjMetadataSet{o1, o2, o3, o4, o5},
			edges:    []Edge{e1, e2, e5, e8},
			expected: []object.ObjMetadataSet{{o5, o3}, {o4}, {o2}, {o1}},
		},
		"simple cycle in graph is an error": {
			vertices: object.ObjMetadataSet{o1, o2},
			edges:    []Edge{e1, e6},
			expected: []object.ObjMetadataSet{},
			expectedError: validation.NewError(
				CyclicDependencyError{
					Edges: []Edge{
						{
							From: o1,
							To:   o2,
						},
						{
							From: o2,
							To:   o1,
						},
					},
				},
				o1, o2,
			),
		},
		"multi-edge cycle in graph is an error": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges:    []Edge{e1, e2, e7},
			expected: []object.ObjMetadataSet{},
			expectedError: validation.NewError(
				CyclicDependencyError{
					Edges: []Edge{
						{
							From: o1,
							To:   o2,
						},
						{
							From: o2,
							To:   o3,
						},
						{
							From: o3,
							To:   o1,
						},
					},
				},
				o1, o2, o3,
			),
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
			if tc.expectedError != nil {
				assert.EqualError(t, tc.expectedError, err.Error())
				return
			}
			assert.NoError(t, err)
			testutil.AssertEqual(t, tc.expected, actual)

			// verify sort is repeatable & non-destructive
			actual, err = g.Sort()
			assert.NoError(t, err)
			testutil.AssertEqual(t, tc.expected, actual)
		})
	}
}

func TestGraphDependencies(t *testing.T) {
	testCases := map[string]struct {
		vertices object.ObjMetadataSet
		edges    []Edge
		from     object.ObjMetadata
		expected object.ObjMetadataSet
	}{
		"no dependencies": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges: []Edge{
				{From: o1, To: o2},
				{From: o1, To: o3},
				{From: o2, To: o3},
			},
			from:     o3,
			expected: object.ObjMetadataSet{},
		},
		"one dependency": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges: []Edge{
				{From: o1, To: o2},
				{From: o1, To: o3},
				{From: o2, To: o3},
			},
			from:     o2,
			expected: object.ObjMetadataSet{o3},
		},
		"two dependencies": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges: []Edge{
				{From: o1, To: o2},
				{From: o1, To: o3},
				{From: o2, To: o3},
			},
			from:     o1,
			expected: object.ObjMetadataSet{o2, o3},
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

			testutil.AssertEqual(t, tc.expected, g.Dependencies(tc.from))
		})
	}
}

func TestGraphDependents(t *testing.T) {
	testCases := map[string]struct {
		vertices object.ObjMetadataSet
		edges    []Edge
		to       object.ObjMetadata
		expected object.ObjMetadataSet
	}{
		"no dependents": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges: []Edge{
				{From: o1, To: o2},
				{From: o1, To: o3},
				{From: o2, To: o3},
			},
			to:       o1,
			expected: object.ObjMetadataSet{},
		},
		"one dependent": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges: []Edge{
				{From: o1, To: o2},
				{From: o1, To: o3},
				{From: o2, To: o3},
			},
			to:       o2,
			expected: object.ObjMetadataSet{o1},
		},
		"two dependents": {
			vertices: object.ObjMetadataSet{o1, o2, o3},
			edges: []Edge{
				{From: o1, To: o2},
				{From: o1, To: o3},
				{From: o2, To: o3},
			},
			to:       o3,
			expected: object.ObjMetadataSet{o1, o2},
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

			testutil.AssertEqual(t, tc.expected, g.Dependents(tc.to))
		})
	}
}

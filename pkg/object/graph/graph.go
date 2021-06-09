// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// This package provides a graph data struture
// and graph functionality using ObjMetadata as
// vertices in the graph.
package graph

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/object"
)

// Graph is contains a directed set of edges, implemented as
// an adjacency list (map key is "from" vertex, slice are "to"
// vertices).
type Graph struct {
	// map "from" vertex -> list of "to" vertices
	edges map[object.ObjMetadata][]object.ObjMetadata
}

// Edge encapsulates a pair of vertices describing a
// directed edge.
type Edge struct {
	From object.ObjMetadata
	To   object.ObjMetadata
}

// New returns a pointer to an empty Graph data structure.
func New() *Graph {
	g := &Graph{}
	g.edges = make(map[object.ObjMetadata][]object.ObjMetadata)
	return g
}

// AddVertex adds an ObjMetadata vertex to the graph, with
// an initial empty set of edges from added vertex.
func (g *Graph) AddVertex(v object.ObjMetadata) {
	if _, exists := g.edges[v]; !exists {
		g.edges[v] = []object.ObjMetadata{}
	}
}

// AddEdge adds a edge from one ObjMetadata vertex to another. The
// direction of the edge is "from" -> "to".
func (g *Graph) AddEdge(from object.ObjMetadata, to object.ObjMetadata) {
	// Add "from" vertex if it doesn't already exist.
	if _, exists := g.edges[from]; !exists {
		g.edges[from] = []object.ObjMetadata{}
	}
	// Add "to" vertex if it doesn't already exist.
	if _, exists := g.edges[to]; !exists {
		g.edges[to] = []object.ObjMetadata{}
	}
	// Add edge "from" -> "to" if it doesn't already exist
	// into the adjacency list.
	if !g.isAdjacent(from, to) {
		g.edges[from] = append(g.edges[from], to)
	}
}

// GetEdges returns the slice of vertex pairs which are
// the directed edges of the graph.
func (g *Graph) GetEdges() []Edge {
	edges := []Edge{}
	for from, toList := range g.edges {
		for _, to := range toList {
			edge := Edge{From: from, To: to}
			edges = append(edges, edge)
		}
	}
	return edges
}

// isAdjacent returns true if an edge "from" vertex -> "to" vertex exists;
// false otherwise.
func (g *Graph) isAdjacent(from object.ObjMetadata, to object.ObjMetadata) bool {
	// If "from" vertex does not exist, it is impossible edge exists; return false.
	if _, exists := g.edges[from]; !exists {
		return false
	}
	// Iterate through adjacency list to see if "to" vertex is adjacent.
	for _, vertex := range g.edges[from] {
		if vertex == to {
			return true
		}
	}
	return false
}

// Size returns the number of vertices in the graph.
func (g *Graph) Size() int {
	return len(g.edges)
}

// removeVertex removes the passed vertex as well as any edges
// into the vertex.
func (g *Graph) removeVertex(r object.ObjMetadata) {
	// First, remove the object from all adjacency lists.
	for v, adj := range g.edges {
		for i, a := range adj {
			if a == r {
				g.edges[v] = removeObj(adj, i)
				break
			}
		}
	}
	// Finally, remove the vertex
	delete(g.edges, r)
}

// removeObj removes the object at index "i" from the passed
// list of vertices, returning the new list.
func removeObj(adj []object.ObjMetadata, i int) []object.ObjMetadata {
	adj[len(adj)-1], adj[i] = adj[i], adj[len(adj)-1]
	return adj[:len(adj)-1]
}

// Sort returns the ordered set of vertices after
// a topological sort.
func (g *Graph) Sort() ([][]object.ObjMetadata, error) {
	sorted := [][]object.ObjMetadata{}
	for g.Size() > 0 {
		// Identify all the leaf vertices.
		leafVertices := []object.ObjMetadata{}
		for v, adj := range g.edges {
			if len(adj) == 0 {
				leafVertices = append(leafVertices, v)
			}
		}
		// No leaf vertices means cycle in the directed graph.
		if len(leafVertices) == 0 {
			return sorted, fmt.Errorf("cycle in directed graph")
		}
		// Remove all edges to leaf vertices.
		for _, v := range leafVertices {
			g.removeVertex(v)
		}
		sorted = append(sorted, leafVertices)
	}
	return sorted, nil
}

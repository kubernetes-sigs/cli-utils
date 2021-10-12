// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// This package provides a object sorting functionality
// based on the explicit "depends-on" annotation, and
// implicit object dependencies like namespaces and CRD's.
package graph

import (
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/dependson"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
)

// SortObjs returns a slice of the sets of objects to apply (in order).
// Each of the objects in an apply set is applied together. The order of
// the returned applied sets is a topological ordering of the sets to apply.
// Returns an single empty apply set if there are no objects to apply.
func SortObjs(objs []*unstructured.Unstructured) ([][]*unstructured.Unstructured, error) {
	if len(objs) == 0 {
		return [][]*unstructured.Unstructured{}, nil
	}
	// Create the graph, and build a map of object metadata to the object (Unstructured).
	g := New()
	objToUnstructured := map[object.ObjMetadata]*unstructured.Unstructured{}
	for _, obj := range objs {
		id := object.UnstructuredToObjMetaOrDie(obj)
		objToUnstructured[id] = obj
	}
	// Add object vertices and dependency edges to graph.
	addApplyTimeMutationEdges(g, objs)
	addDependsOnEdges(g, objs)
	addNamespaceEdges(g, objs)
	addCRDEdges(g, objs)
	addGateKeeperEdges(g, objs)
	// Run topological sort on the graph.
	objSets := [][]*unstructured.Unstructured{}
	sortedObjSets, err := g.Sort()
	if err != nil {
		return [][]*unstructured.Unstructured{}, err
	}
	// Map the object metadata back to the sorted sets of unstructured objects.
	for _, objSet := range sortedObjSets {
		currentSet := []*unstructured.Unstructured{}
		for _, id := range objSet {
			var found bool
			var obj *unstructured.Unstructured
			if obj, found = objToUnstructured[id]; found {
				currentSet = append(currentSet, obj)
			}
		}
		objSets = append(objSets, currentSet)
	}
	return objSets, nil
}

// ReverseSortObjs is the same as SortObjs but using reverse ordering.
func ReverseSortObjs(objs []*unstructured.Unstructured) ([][]*unstructured.Unstructured, error) {
	// Sorted objects using normal ordering.
	s, err := SortObjs(objs)
	if err != nil {
		return [][]*unstructured.Unstructured{}, err
	}
	// Reverse the ordering of the object sets using swaps.
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s, nil
}

// addApplyTimeMutationEdges updates the graph with edges from objects
// with an explicit "apply-time-mutation" annotation.
func addApplyTimeMutationEdges(g *Graph, objs []*unstructured.Unstructured) {
	for _, obj := range objs {
		id := object.UnstructuredToObjMetaOrDie(obj)
		klog.V(3).Infof("adding vertex: %s", id)
		g.AddVertex(id)
		if mutation.HasAnnotation(obj) {
			subs, err := mutation.ReadAnnotation(obj)
			if err != nil {
				// TODO: fail task if parse errors?
				klog.V(3).Infof("failed to add edges from: %s: %s", id, err)
				return
			}
			for _, sub := range subs {
				// TODO: fail task if it's not in the inventory?
				dep := sub.SourceRef.ObjMetadata()
				klog.V(3).Infof("adding edge from: %s, to: %s", id, dep)
				g.AddEdge(id, dep)
			}
		}
	}
}

// addDependsOnEdges updates the graph with edges from objects
// with an explicit "depends-on" annotation.
func addDependsOnEdges(g *Graph, objs []*unstructured.Unstructured) {
	for _, obj := range objs {
		id := object.UnstructuredToObjMetaOrDie(obj)
		klog.V(3).Infof("adding vertex: %s", id)
		g.AddVertex(id)
		deps, err := dependson.ReadAnnotation(obj)
		if err != nil {
			// TODO: fail if annotation fails to parse?
			klog.V(3).Infof("failed to add edges from: %s: %s", id, err)
			continue
		}
		for _, dep := range deps {
			// TODO: fail if depe is not in the inventory?
			klog.V(3).Infof("adding edge from: %s, to: %s", id, dep)
			g.AddEdge(id, dep)
		}
	}
}

// addCRDEdges adds edges to the dependency graph from custom
// resources to their definitions to ensure the CRD's exist
// before applying the custom resources created with the definition.
func addCRDEdges(g *Graph, objs []*unstructured.Unstructured) {
	crds := map[string]object.ObjMetadata{}
	// First create a map of all the CRD's.
	for _, u := range objs {
		if object.IsCRD(u.GroupVersionKind().GroupKind()) {
			groupKind, found := object.GetCRDGroupKind(u)
			if found {
				obj := object.UnstructuredToObjMetaOrDie(u)
				crds[groupKind.String()] = obj
			}
		}
	}
	// Iterate through all resources to see if we are applying any
	// custom resources defined by previously recorded CRD's.
	for _, u := range objs {
		gvk := u.GroupVersionKind()
		groupKind := gvk.GroupKind()
		if to, found := crds[groupKind.String()]; found {
			from := object.UnstructuredToObjMetaOrDie(u)
			klog.V(3).Infof("adding edge from: custom resource %s, to CRD: %s", from, to)
			g.AddEdge(from, to)
		}
	}
}

func addGateKeeperEdges(g *Graph, objs []*unstructured.Unstructured) {
	fmt.Println("checking for GateKeeper edges")
	templates := make(map[schema.GroupKind]object.ObjMetadata)
	for _, u := range objs {
		if object.IsConstraintTemplate(u.GroupVersionKind().GroupKind()) {
			groupKind, found := object.GetConstraintGroupKind(u)
			if found {
				fmt.Printf("Found constraint template: %s\n", groupKind.String())
				obj := object.UnstructuredToObjMetaOrDie(u)
				templates[groupKind] = obj
			}
		}
	}

	for _, u := range objs {
		gvk := u.GroupVersionKind()
		groupKind := gvk.GroupKind()
		if to, found := templates[groupKind]; found {
			from := object.UnstructuredToObjMetaOrDie(u)
			klog.V(3).Infof("adding edge from: constraint %s, to template: %s", from, to)
			g.AddEdge(from, to)
		}
	}
}

// addNamespaceEdges adds edges to the dependency graph from namespaced
// objects to the namespace objects. Ensures the namespaces exist
// before the resources in those namespaces are applied.
func addNamespaceEdges(g *Graph, objs []*unstructured.Unstructured) {
	namespaces := map[string]object.ObjMetadata{}
	// First create a map of all the namespaces objects live in.
	for _, obj := range objs {
		if object.IsKindNamespace(obj) {
			id := object.UnstructuredToObjMetaOrDie(obj)
			namespace := obj.GetName()
			namespaces[namespace] = id
		}
	}
	// Next, if the namespace of a namespaced object is being applied,
	// then create an edge from the namespaced object to its namespace.
	for _, obj := range objs {
		if object.IsNamespaced(obj) {
			objNamespace := obj.GetNamespace()
			if namespace, found := namespaces[objNamespace]; found {
				id := object.UnstructuredToObjMetaOrDie(obj)
				klog.V(3).Infof("adding edge from: %s to namespace: %s", id, namespace)
				g.AddEdge(id, namespace)
			}
		}
	}
}

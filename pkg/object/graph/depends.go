// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// This package provides a object sorting functionality
// based on the explicit "depends-on" annotation, and
// implicit object dependencies like namespaces and CRD's.
package graph

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/dependson"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
)

// SortObjs returns two topologically ordered lists of sets of objects:
// - objects to apply
// - objects to wait on for reconciliation
//
// The objects in each set are independent of other objects in the same set.
//
// Returns nil if no objects are supplied.
func SortObjs(objs []*unstructured.Unstructured) (
	[]object.UnstructuredSet,
	[]object.ObjMetadataSet,
	error,
) {
	objSets := []object.UnstructuredSet(nil)
	externalDepSets := []object.ObjMetadataSet(nil)
	if len(objs) == 0 {
		return objSets, externalDepSets, nil
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
	// Run topological sort on the graph.
	sortedObjSets, err := g.Sort()
	if err != nil {
		return objSets, externalDepSets, err
	}
	// Map the object metadata back to the sorted sets of unstructured objects.
	for _, objSet := range sortedObjSets {
		currentObjSet := object.UnstructuredSet{}
		currentDepSet := object.ObjMetadataSet{}
		for _, id := range objSet {
			obj, found := objToUnstructured[id]
			if found {
				currentObjSet = append(currentObjSet, obj)
			} else {
				currentDepSet = append(currentDepSet, id)
			}
		}
		objSets = append(objSets, currentObjSet)
		externalDepSets = append(externalDepSets, currentDepSet)
	}
	return objSets, externalDepSets, nil
}

// ReverseSortObjs returns two topologically ordered lists of sets of objects:
// - objects to delete
// - objects to wait on for deletion
//
// The objects in each set are independent of other objects in the same set.
//
// Returns nil if no objects are supplied.
func ReverseSortObjs(objs []*unstructured.Unstructured) (
	[]object.UnstructuredSet,
	[]object.ObjMetadataSet,
	error,
) {
	// Sorted objects using normal ordering.
	s, d, err := SortObjs(objs)
	if err != nil {
		return nil, nil, err
	}
	// Reverse the ordering of the object sets using swaps.
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
		d[i], d[j] = d[j], d[i]
	}
	return s, d, nil
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
				dep := sub.SourceRef.ObjMetadata()
				// TODO: only set default namespace if resource is namespaced (look up mapping)
				if dep.Namespace == "" {
					dep.Namespace = obj.GetNamespace()
				}
				klog.V(3).Infof("adding vertex: %s", dep)
				g.AddVertex(dep)
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
			klog.V(3).Infof("adding vertex: %s", dep)
			g.AddVertex(dep)
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
		if object.IsCRD(u) {
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

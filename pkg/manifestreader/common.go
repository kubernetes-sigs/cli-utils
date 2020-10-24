// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// SetNamespaces verifies that every namespaced resource has the namespace
// set, and if one does not, it will set the namespace to the provided
// defaultNamespace.
// This implementation will check each resource (that doesn't already have
// the namespace set) on whether it is namespace or cluster scoped. It does
// this by first checking the RESTMapper, and it there is not match there,
// it will look for CRDs in the provided Unstructureds.
func SetNamespaces(mapper meta.RESTMapper, objs []*unstructured.Unstructured,
	defaultNamespace string, enforceNamespace bool) error {
	var crdObjs []*unstructured.Unstructured

	// find any crds in the set of resources.
	for _, obj := range objs {
		if solver.IsCRD(obj) {
			crdObjs = append(crdObjs, obj)
		}
	}

	for _, obj := range objs {
		accessor, _ := meta.Accessor(obj)

		// Exclude any inventory objects here since we don't want to change
		// their namespace.
		if inventory.IsInventoryObject(obj) {
			continue
		}

		// if the resource already has the namespace set, we don't
		// need to do anything
		if ns := accessor.GetNamespace(); ns != "" {
			if enforceNamespace && ns != defaultNamespace {
				return fmt.Errorf("the namespace from the provided object %q "+
					"does not match the namespace %q. You must pass '--namespace=%s' to perform this operation",
					ns, defaultNamespace, ns)
			}
			continue
		}

		gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
		mapping, err := mapper.RESTMapping(gk)

		if err != nil && !meta.IsNoMatchError(err) {
			return err
		}

		if err == nil {
			// If we find a mapping for the resource type in the RESTMapper,
			// we just use it.
			if mapping.Scope == meta.RESTScopeNamespace {
				// This means the resource does not have the namespace set,
				// but it is a namespaced resource. So we set the namespace
				// to the provided default value.
				accessor.SetNamespace(defaultNamespace)
			}
			continue
		}

		// If we get here, it means the resource does not have the namespace
		// set and we didn't find the resource type in the RESTMapper. As
		// a last try, we look at all the CRDS that are part of the set and
		// see if we get a match on the resource type. If so, we can determine
		// from the CRD whether the resource type is cluster-scoped or
		// namespace-scoped. If it is the latter, we set the namespace
		// to the provided default.
		var scope string
		for _, crdObj := range crdObjs {
			group, _, _ := unstructured.NestedString(crdObj.Object, "spec", "group")
			kind, _, _ := unstructured.NestedString(crdObj.Object, "spec", "names", "kind")
			if gk.Kind == kind && gk.Group == group {
				scope, _, _ = unstructured.NestedString(crdObj.Object, "spec", "scope")
			}
		}

		switch scope {
		case "":
			return fmt.Errorf("can't find scope for resource %s %s", gk.String(), accessor.GetName())
		case "Cluster":
			continue
		case "Namespaced":
			accessor.SetNamespace(defaultNamespace)
		}
	}

	return nil
}

// FilterLocalConfig returns a new slice of Unstructured where all resources
// with the LocalConfig annotation is filtered out.
func FilterLocalConfig(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	var filteredObjs []*unstructured.Unstructured
	for _, obj := range objs {
		acc, _ := meta.Accessor(obj)
		// Ignoring the value of the LocalConfigAnnotation here. This is to be
		// consistent with the behavior in the kyaml library:
		// https://github.com/kubernetes-sigs/kustomize/blob/30b58e90a39485bc5724b2278651c5d26b815cb2/kyaml/kio/filters/local.go#L29
		if _, found := acc.GetAnnotations()[filters.LocalConfigAnnotation]; !found {
			filteredObjs = append(filteredObjs, obj)
		}
	}
	return filteredObjs
}

// removeAnnotations removes the specified kioutil annotations from the resource.
func removeAnnotations(n *yaml.RNode, annotations ...kioutil.AnnotationKey) error {
	for _, a := range annotations {
		err := n.PipeE(yaml.ClearAnnotation(a))
		if err != nil {
			return err
		}
	}
	return nil
}

// kyamlNodeToUnstructured take a resource represented as a kyaml RNode and
// turns it into an Unstructured object.
func kyamlNodeToUnstructured(n *yaml.RNode) (*unstructured.Unstructured, error) {
	b, err := n.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	err = json.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{
		Object: m,
	}, nil
}

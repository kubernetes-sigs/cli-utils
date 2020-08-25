// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package manifestreader

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

// ManifestReader defines the interface for reading a set
// of manifests into info objects.
type ManifestReader interface {
	Read() ([]*resource.Info, error)
}

// ReaderOptions defines the shared inputs for the different
// implementations of the ManifestReader interface.
type ReaderOptions struct {
	Factory          util.Factory
	Validate         bool
	Namespace        string
	EnforceNamespace bool
}

// setNamespaces verifies that every namespaced resource has the namespace
// set, and if one does not, it will set the namespace to the provided
// defaultNamespace.
// This implementation will check each resource (that doesn't already have
// the namespace set) on whether it is namespace or cluster scoped. It does
// this by first checking the RESTMapper, and it there is not match there,
// it will look for CRDs in the provided Infos.
func setNamespaces(factory util.Factory, infos []*resource.Info,
	defaultNamespace string, enforceNamespace bool) error {
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return err
	}

	var crdInfos []*resource.Info

	// find any crds in the set of resources.
	for _, inf := range infos {
		if solver.IsCRD(inf) {
			crdInfos = append(crdInfos, inf)
		}
	}

	for _, inf := range infos {
		accessor, _ := meta.Accessor(inf.Object)

		// Exclude any inventory objects here since we don't want to change
		// their namespace.
		if inventory.IsInventoryObject(inf) {
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

		gk := inf.Object.GetObjectKind().GroupVersionKind().GroupKind()
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
				inf.Namespace = defaultNamespace
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
		for _, crdInf := range crdInfos {
			u, _ := crdInf.Object.(*unstructured.Unstructured)
			group, _, _ := unstructured.NestedString(u.Object, "spec", "group")
			kind, _, _ := unstructured.NestedString(u.Object, "spec", "names", "kind")
			if gk.Kind == kind && gk.Group == group {
				scope, _, _ = unstructured.NestedString(u.Object, "spec", "scope")
			}
		}

		switch scope {
		case "":
			return fmt.Errorf("can't find scope for resource %s %s", gk.String(), accessor.GetName())
		case "Cluster":
			continue
		case "Namespaced":
			inf.Namespace = defaultNamespace
			accessor.SetNamespace(defaultNamespace)
		}
	}

	return nil
}

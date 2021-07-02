// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package object

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	CoreNamespace   = CoreV1Namespace.GroupKind()
	CoreV1Namespace = corev1.SchemeGroupVersion.WithKind("Namespace")
	ExtensionsCRD   = ExtensionsV1CRD.GroupKind()
	ExtensionsV1CRD = extensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition")
)

// UnstructuredsToObjMetas converts a slice of unstructureds to a slice of
// ObjMetadata. If the values for any of the unstructured objects doesn't
// pass validation, an error will be returned.
func UnstructuredsToObjMetas(objs []*unstructured.Unstructured) ([]ObjMetadata, error) {
	objMetas := make([]ObjMetadata, 0, len(objs))
	for _, obj := range objs {
		objMeta, err := UnstructuredToObjMeta(obj)
		if err != nil {
			return nil, err
		}
		objMetas = append(objMetas, objMeta)
	}
	return objMetas, nil
}

// UnstructuredsToObjMetasOrDie converts a slice of unstructureds to a slice of
// ObjMetadata. If the values for any of the unstructured objects doesn't
// pass validation, the function will panic.
func UnstructuredsToObjMetasOrDie(objs []*unstructured.Unstructured) []ObjMetadata {
	objMetas, err := UnstructuredsToObjMetas(objs)
	if err != nil {
		panic(err)
	}
	return objMetas
}

// UnstructuredToObjMeta extracts the identifying information from an
// Unstructured object and returns it as Objmetadata. If the values doesn't
// pass validation, an error will be returned.
func UnstructuredToObjMeta(obj *unstructured.Unstructured) (ObjMetadata, error) {
	return CreateObjMetadata(obj.GetNamespace(), obj.GetName(),
		obj.GroupVersionKind().GroupKind())
}

// UnstructuredToObjMetaOrDie extracts the identifying information from an
// Unstructured object and returns it as Objmetadata. If the values doesn't
// pass validation, the function will panic.
func UnstructuredToObjMetaOrDie(obj *unstructured.Unstructured) ObjMetadata {
	objMeta, err := UnstructuredToObjMeta(obj)
	if err != nil {
		panic(err)
	}
	return objMeta
}

// IsKindNamespace returns true if the passed Unstructured object is
// GroupKind == Core/Namespace (no version checked); false otherwise.
func IsKindNamespace(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	gvk := u.GroupVersionKind()
	return CoreNamespace == gvk.GroupKind()
}

// IsNamespaced returns true if the passed Unstructured object
// is namespace-scoped (not cluster-scoped); false otherwise.
func IsNamespaced(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	return u.GetNamespace() != ""
}

// IsCRD returns true if the passed Unstructured object has
// GroupKind == Extensions/CustomResourceDefinition; false otherwise.
func IsCRD(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	gvk := u.GroupVersionKind()
	return ExtensionsCRD == gvk.GroupKind()
}

// GetCRDGroupKind returns the GroupKind stored in the passed
// Unstructured CustomResourceDefinition and true if the passed object
// is a CRD.
func GetCRDGroupKind(u *unstructured.Unstructured) (schema.GroupKind, bool) {
	emptyGroupKind := schema.GroupKind{Group: "", Kind: ""}
	if u == nil {
		return emptyGroupKind, false
	}
	group, found, err := unstructured.NestedString(u.Object, "spec", "group")
	if found && err == nil {
		kind, found, err := unstructured.NestedString(u.Object, "spec", "names", "kind")
		if found && err == nil {
			return schema.GroupKind{Group: group, Kind: kind}, true
		}
	}
	return emptyGroupKind, false
}

// UnknownTypeError captures information about a type for which no information
// could be found in the cluster or among the known CRDs.
type UnknownTypeError struct {
	GroupKind schema.GroupKind
}

func (e *UnknownTypeError) Error() string {
	return fmt.Sprintf("unknown resource type: %q", e.GroupKind.String())
}

// LookupResourceScope tries to look up the scope of the type of the provided
// resource, looking at both the types known to the cluster (through the
// RESTMapper) and the provided CRDs. If no information about the type can
// be found, an UnknownTypeError wil be returned.
func LookupResourceScope(u *unstructured.Unstructured, crds []*unstructured.Unstructured, mapper meta.RESTMapper) (meta.RESTScope, error) {
	gvk := u.GroupVersionKind()
	// First see if we can find the type (and the scope) in the cluster through
	// the RESTMapper.
	mapping, err := mapper.RESTMapping(gvk.GroupKind())
	// Not finding a match is not an error here, so only error out for other
	// error types.
	if err != nil && !meta.IsNoMatchError(err) {
		return nil, err
	}

	var scope meta.RESTScope
	if err == nil {
		// If we find the type in the cluster, we just look up the scope there.
		scope = mapping.Scope
	} else {
		// If we couldn't find the type in the cluster, check if we find a
		// match in any of the provided CRDs.
		for _, crd := range crds {
			group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
			kind, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "kind")
			if gvk.Kind == kind && gvk.Group == group {
				scopeName, _, _ := unstructured.NestedString(crd.Object, "spec", "scope")
				switch scopeName {
				case "Namespaced":
					scope = meta.RESTScopeNamespace
				case "Cluster":
					scope = meta.RESTScopeRoot
				default:
					return nil, fmt.Errorf("unknown scope %q", scopeName)
				}
				break
			}
		}
	}

	if scope == nil {
		return nil, &UnknownTypeError{
			GroupKind: gvk.GroupKind(),
		}
	}
	return scope, nil
}

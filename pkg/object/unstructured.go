// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package object

import (
	corev1 "k8s.io/api/core/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	CoreNamespace   = CoreV1Namespace.GroupKind()
	CoreV1Namespace = corev1.SchemeGroupVersion.WithKind("Namespace")
	ExtensionsCRD   = ExtensionsV1CRD.GroupKind()
	ExtensionsV1CRD = extensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition")
)

// UnstructuredsToObjMetas returns a slice of ObjMetadata translated from
// a slice of Unstructured objects.
func UnstructuredsToObjMetas(objs []*unstructured.Unstructured) []ObjMetadata {
	objMetas := make([]ObjMetadata, 0, len(objs))
	for _, obj := range objs {
		objMetas = append(objMetas, ObjMetadata{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			GroupKind: obj.GroupVersionKind().GroupKind(),
		})
	}
	return objMetas
}

// UnstructuredsToObjMetas returns an ObjMetadata translated from
// an Unstructured object.
func UnstructuredToObjMeta(obj *unstructured.Unstructured) ObjMetadata {
	return ObjMetadata{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		GroupKind: obj.GroupVersionKind().GroupKind(),
	}
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

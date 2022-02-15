// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ObjMetadataEqualObjectReference compares an ObjMetadata with a ObjectReference
func ObjMetadataEqualObjectReference(id object.ObjMetadata, ref actuation.ObjectReference) bool {
	return id.GroupKind.Group == ref.Group &&
		id.GroupKind.Kind == ref.Kind &&
		id.Namespace == ref.Namespace &&
		id.Name == ref.Name
}

// ObjectReferenceFromObjMetadata converts an ObjMetadata to a ObjectReference
func ObjectReferenceFromObjMetadata(id object.ObjMetadata) actuation.ObjectReference {
	return actuation.ObjectReference{
		Group:     id.GroupKind.Group,
		Kind:      id.GroupKind.Kind,
		Name:      id.Name,
		Namespace: id.Namespace,
	}
}

// ObjMetadataFromObjectReference converts an ObjectReference to a ObjMetadata
func ObjMetadataFromObjectReference(ref actuation.ObjectReference) object.ObjMetadata {
	return object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: ref.Group,
			Kind:  ref.Kind,
		},
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}
}

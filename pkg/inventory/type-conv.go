// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		GroupKind: GroupKindFromObjectReference(ref),
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}
}

// ObjMetadataSetFromObjectReferences converts an []ObjectReference to a ObjMetadataSet
func ObjMetadataSetFromObjectReferences(refs []actuation.ObjectReference) object.ObjMetadataSet {
	ids := make(object.ObjMetadataSet, len(refs))
	for i, ref := range refs {
		ids[i] = ObjMetadataFromObjectReference(ref)
	}
	return ids
}

// ObjectReferencesFromObjMetadataSet converts an ObjMetadataSet to a []ObjectReference
func ObjectReferencesFromObjMetadataSet(ids object.ObjMetadataSet) []actuation.ObjectReference {
	refs := make([]actuation.ObjectReference, len(ids))
	for i, id := range ids {
		refs[i] = ObjectReferenceFromObjMetadata(id)
	}
	return refs
}

// ObjectReferenceFromObject builds a reference to an object (structured or
// unstrutured).
func ObjectReferenceFromObject(obj client.Object) actuation.ObjectReference {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return actuation.ObjectReference{
		Group:     gvk.Group,
		Kind:      gvk.Kind,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}

// InfoFromObject builds an InventoryInfo from an inventory object
// (structured or unstrutured).
func InfoFromObject(obj client.Object) Info {
	return Info{
		ObjectReference: ObjectReferenceFromObject(obj),
		ID:              Label(obj),
	}
}

func GroupKindFromObjectReference(ref actuation.ObjectReference) schema.GroupKind {
	return schema.GroupKind{
		Group: ref.Group,
		Kind:  ref.Kind,
	}
}

func NewObjectReferenceStringer(ref actuation.ObjectReference) ObjectReferenceStringer {
	return ObjectReferenceStringer{
		Ref: ref,
	}
}

type ObjectReferenceStringer struct {
	Ref actuation.ObjectReference
}

func (ors ObjectReferenceStringer) String() string {
	if ors.Ref.Namespace != "" {
		return fmt.Sprintf("%s/namespaces/%s/%s/%s",
			ors.Ref.Group, ors.Ref.Namespace, ors.Ref.Kind, ors.Ref.Name)
	}
	return fmt.Sprintf("%s/%s/%s", ors.Ref.Group, ors.Ref.Kind, ors.Ref.Name)
}

func NewInfoStringer(info Info) InfoStringer {
	return InfoStringer{
		Info: info,
	}
}

type InfoStringer struct {
	Info Info
}

func (ors InfoStringer) String() string {
	return fmt.Sprintf("{ref: %s, id: %s}",
		NewObjectReferenceStringer(ors.Info.ObjectReference), ors.Info.ID)
}

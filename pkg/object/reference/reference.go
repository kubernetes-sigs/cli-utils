// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package reference

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
)

const (
	// Used to separate the fields for a depends-on object value.
	segmentDelimiter  = "/"
	namespacesSegment = "namespaces"
)

// ObjectReference is a reference to a KRM resource by name and kind.
// One of APIVersion or Group is required.
// Group is generally preferred, to avoid needing to update the version in lock
// step with the referenced resource.
// If neither is provided, the empty group is used.
type ObjectReference struct {
	// Kind is a string value representing the REST resource this object represents.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind"`

	// APIVersion defines the versioned schema of this representation of an object.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Group is accepted as a version-less alternative to APIVersion
	// More info: https://kubernetes.io/docs/reference/using-api/#api-groups
	// +optional
	Group string `json:"group,omitempty"`

	// Name of the resource.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name,omitempty"`

	// Namespace is optional, defaults to the namespace of the target resource.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ObjectReferenceFromUnstructured returns the object as a ObjectReference
func ObjectReferenceFromUnstructured(obj *unstructured.Unstructured) ObjectReference {
	return ObjectReference{
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Kind:       obj.GetKind(),
		APIVersion: obj.GetAPIVersion(),
	}
}

// ObjectReferenceFromObjMetadata returns the object as a ObjectReference
func ObjectReferenceFromObjMetadata(id object.ObjMetadata) ObjectReference {
	return ObjectReference{
		Name:      id.Name,
		Namespace: id.Namespace,
		Kind:      id.GroupKind.Kind,
		Group:     id.GroupKind.Group,
	}
}

// GroupVersionKind satisfies the ObjectKind interface for all objects that
// embed TypeMeta. Prefers Group over APIVersion.
func (r ObjectReference) GroupVersionKind() schema.GroupVersionKind {
	if r.Group != "" {
		return schema.GroupVersionKind{Group: r.Group, Kind: r.Kind}
	}
	return schema.FromAPIVersionAndKind(r.APIVersion, r.Kind)
}

// ToUnstructured returns the name, namespace, group, version, and kind of the
// ObjectReference, wrapped in a new Unstructured object.
// This is useful for performing operations with
// sigs.k8s.io/controller-runtime/pkg/client's unstructured Client.
func (r ObjectReference) ToUnstructured() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(r.Name)
	obj.SetNamespace(r.Namespace)
	obj.SetGroupVersionKind(r.GroupVersionKind())
	return obj
}

// ToUnstructured returns the name, namespace, group, and kind of the
// ObjectReference, wrapped in a new ObjMetadata object.
func (r ObjectReference) ToObjMetadata() object.ObjMetadata {
	return object.ObjMetadata{
		Namespace: r.Namespace,
		Name:      r.Name,
		GroupKind: r.GroupVersionKind().GroupKind(),
	}
}

// String returns the format GROUP[/VERSION][/namespaces/NAMESPACE]/KIND/NAME
func (r ObjectReference) String() string {
	group := r.Group
	if group == "" {
		group = r.APIVersion
	}
	if r.Namespace != "" {
		return fmt.Sprintf("%s/namespaces/%s/%s/%s", group, r.Namespace, r.Kind, r.Name)
	}
	return fmt.Sprintf("%s/%s/%s", group, r.Kind, r.Name)
}

// Equal returns true if the ObjectReference sets are equal.
// Fulfills Equal interface from github.com/google/go-cmp
func (r ObjectReference) Equal(b ObjectReference) bool {
	return r.GroupVersionKind() == b.GroupVersionKind() &&
		r.Name == b.Name &&
		r.Namespace == b.Namespace
}

// ParseObjectReference parses a string into an ObjectReference.
//
// Object references use the following format:
//   GROUP[/VERSION][/namespaces/NAMESPACE]/KIND/NAME
//
// Segments are separated by '/'.
// Square brackets ([]) indicate optional segments.
//
// Group may be the empty string, but version, namespace, name, and kind may not.
func ParseObjectReference(in string) (objRef ObjectReference, err error) {
	in = strings.TrimSpace(in)
	s := strings.Split(in, segmentDelimiter)
	switch len(s) {
	case 3: // group/kind/name
		objRef = ObjectReference{
			Group: s[0],
			Kind:  s[1],
			Name:  s[2],
		}
	case 4: // group/version/kind/name
		objRef = ObjectReference{
			APIVersion: s[0] + segmentDelimiter + s[1],
			Kind:       s[2],
			Name:       s[3],
		}
	case 5: // group/namespaces/namespace/kind/name
		if s[1] != namespacesSegment {
			err = fmt.Errorf("missing %q segment: %q", namespacesSegment, in)
			break
		}
		objRef = ObjectReference{
			Group:     s[0],
			Kind:      s[3],
			Name:      s[4],
			Namespace: s[2],
		}
	case 6: // group/version/namespaces/namespace/kind/name
		if s[2] != namespacesSegment {
			err = fmt.Errorf("missing %q segment: %q", namespacesSegment, in)
			break
		}
		objRef = ObjectReference{
			APIVersion: s[0] + segmentDelimiter + s[1],
			Kind:       s[4],
			Name:       s[5],
			Namespace:  s[3],
		}
	default:
		err = fmt.Errorf("wrong number of segments: %q", in)
	}
	return objRef, err
}

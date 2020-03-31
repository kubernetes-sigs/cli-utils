// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// ObjMetadata is the minimal set of information to
// uniquely identify an object. The four fields are:
//
//   Group/Kind (NOTE: NOT version)
//   Namespace
//   Name
//
// We specifically do not use the "version", because
// the APIServer does not recognize a version as a
// different resource. This metadata is used to identify
// resources for pruning and teardown.

package object

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Separates inventory fields. This string is allowable as a
// ConfigMap key, but it is not allowed as a character in
// resource name.
const fieldSeparator = "_"

// ObjMetadata organizes and stores the indentifying information
// for an object. This struct (as a string) is stored in a
// grouping object to keep track of sets of applied objects.
type ObjMetadata struct {
	Namespace string
	Name      string
	GroupKind schema.GroupKind
}

// CreateObjMetadata returns a pointer to an ObjMetadata struct filled
// with the passed values. This function normalizes and validates the
// passed fields and returns an error for bad parameters.
func CreateObjMetadata(namespace string, name string, gk schema.GroupKind) (*ObjMetadata, error) {
	// Namespace can be empty, but name cannot.
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("empty name for inventory object")
	}
	if gk.Empty() {
		return nil, fmt.Errorf("empty GroupKind for inventory object")
	}

	return &ObjMetadata{
		Namespace: strings.TrimSpace(namespace),
		Name:      name,
		GroupKind: gk,
	}, nil
}

// ParseObjMetadata takes a string, splits it into its five fields,
// and returns a pointer to an ObjMetadata struct storing the
// five fields. Example inventory string:
//
//   test-namespace_test-name_apps_ReplicaSet
//
// Returns an error if unable to parse and create the ObjMetadata
// struct.
func ParseObjMetadata(inv string) (*ObjMetadata, error) {
	parts := strings.Split(inv, fieldSeparator)
	if len(parts) == 4 {
		gk := schema.GroupKind{
			Group: strings.TrimSpace(parts[2]),
			Kind:  strings.TrimSpace(parts[3]),
		}
		return CreateObjMetadata(parts[0], parts[1], gk)
	}
	return nil, fmt.Errorf("unable to decode inventory: %s", inv)
}

// Equals compares two ObjMetadata and returns true if they are equal. This does
// not contain any special treatment for the extensions API group.
func (o *ObjMetadata) Equals(other *ObjMetadata) bool {
	if other == nil {
		return false
	}
	return *o == *other
}

// String create a string version of the ObjMetadata struct.
func (o *ObjMetadata) String() string {
	return fmt.Sprintf("%s%s%s%s%s%s%s",
		o.Namespace, fieldSeparator,
		o.Name, fieldSeparator,
		o.GroupKind.Group, fieldSeparator,
		o.GroupKind.Kind)
}

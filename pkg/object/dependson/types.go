// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package dependson

import (
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/reference"
)

// DependencySet is a set of object references.
// When testing equality, order is not importent.
type DependencySet reference.ObjectReferenceSet

// Equal returns true if the DependencySets are equivalent, ignoring order.
// Fulfills Equal interface from github.com/google/go-cmp
func (setA DependencySet) Equal(setB DependencySet) bool {
	return reference.ObjectReferenceSet(setA).Equal(reference.ObjectReferenceSet(setB))
}

// String formats a DependencySet into a string.
func (setA DependencySet) String() string {
	return reference.ObjectReferenceSet(setA).String()
}

// ToObjMetadataSet returns the ObjectReferenceSet as an ObjMetadataSet
func (setA DependencySet) ToObjMetadataSet() object.ObjMetadataSet {
	return reference.ObjectReferenceSet(setA).ToObjMetadataSet()
}

// ParseDependentsSet parses a string into a DependencySet.
func ParseDependentsSet(in string) (DependencySet, error) {
	objRefs, err := reference.ParseObjectReferenceSet(in)
	return DependencySet(objRefs), err
}

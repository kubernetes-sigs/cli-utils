// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package dependents

import (
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/reference"
)

// DependentSet is a set of object references.
// When testing equality, order is not importent.
type DependentSet reference.ObjectReferenceSet

// Equal returns true if the DependentSets are equivalent, ignoring order.
// Fulfills Equal interface from github.com/google/go-cmp
func (setA DependentSet) Equal(setB DependentSet) bool {
	return reference.ObjectReferenceSet(setA).Equal(reference.ObjectReferenceSet(setB))
}

// String formats a DependentSet into a string.
func (setA DependentSet) String() string {
	return reference.ObjectReferenceSet(setA).String()
}

// ToObjMetadataSet returns the ObjectReferenceSet as an ObjMetadataSet
func (setA DependentSet) ToObjMetadataSet() object.ObjMetadataSet {
	return reference.ObjectReferenceSet(setA).ToObjMetadataSet()
}

// ParseDependentsSet parses a string into a DependentSet.
func ParseDependentsSet(in string) (DependentSet, error) {
	objRefs, err := reference.ParseObjectReferenceSet(in)
	return DependentSet(objRefs), err
}

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package reference

import (
	"fmt"
	"strings"

	"sigs.k8s.io/cli-utils/pkg/object"
)

const (
	// Used to separate multiple objects references.
	setDelimiter = ","
)

// ObjectReferenceSet is an ordered list of ObjectReference that acts like an
// unordered set for comparison purposes.
type ObjectReferenceSet []ObjectReference

// Equal returns true if the two sets contain equivalent objects.
// Duplicates are ignored.
// This function satisfies the cmp.Equal interface from github.com/google/go-cmp
func (setA ObjectReferenceSet) Equal(setB ObjectReferenceSet) bool {
	mapA := make(map[ObjectReference]struct{}, len(setA))
	for _, a := range setA {
		mapA[a] = struct{}{}
	}
	mapB := make(map[ObjectReference]struct{}, len(setB))
	for _, b := range setB {
		mapB[b] = struct{}{}
	}
	if len(mapA) != len(mapB) {
		return false
	}
	for b := range mapB {
		if _, exists := mapA[b]; !exists {
			return false
		}
	}
	return true
}

// String returns a comma delimited list of stringified object references.
func (setA ObjectReferenceSet) String() string {
	switch len(setA) {
	case 0:
		return ""
	case 1:
		return setA[0].String()
	default:
		var sb strings.Builder
		sb.WriteString(setA[0].String())
		for _, objRef := range setA[1:] {
			sb.WriteString(setDelimiter)
			sb.WriteString(objRef.String())
		}
		return sb.String()
	}
}

// ToObjMetadataSet returns the ObjectReferenceSet as an ObjMetadataSet
func (setA ObjectReferenceSet) ToObjMetadataSet() object.ObjMetadataSet {
	ids := make(object.ObjMetadataSet, len(setA))
	for i, objRef := range setA {
		ids[i] = objRef.ToObjMetadata()
	}
	return ids
}

// ParseObjectReferenceSet parses the passed string as a set of object
// references.
//
// Object references are delimited by commas (,).
//
// Returns the parsed ObjectReferenceSet or an error if unable to parse.
func ParseObjectReferenceSet(in string) (ObjectReferenceSet, error) {
	objRefs := ObjectReferenceSet{}
	for i, objStr := range strings.Split(in, setDelimiter) {
		obj, err := ParseObjectReference(objStr)
		if err != nil {
			return objRefs, fmt.Errorf("failed to parse object reference (index: %d): %w", i, err)
		}
		objRefs = append(objRefs, obj)
	}
	return objRefs, nil
}

// ObjectReferenceSetFromObjMetadataSet returns the ObjMetadataSet as an
// ObjectReferenceSet
func ObjectReferenceSetFromObjMetadataSet(ids object.ObjMetadataSet) ObjectReferenceSet {
	objRefs := make(ObjectReferenceSet, len(ids))
	for i, id := range ids {
		objRefs[i] = ObjectReferenceFromObjMetadata(id)
	}
	return objRefs
}

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package object

import (
	"hash/fnv"
	"sort"
	"strconv"
)

// ObjMetadataSet is an ordered list of ObjMetadata that acts like an unordered
// set for comparison purposes.
type ObjMetadataSet []ObjMetadata

// UnstructuredSetEquals returns true if the slice of objects in setA equals
// the slice of objects in setB.
func ObjMetadataSetEquals(setA []ObjMetadata, setB []ObjMetadata) bool {
	return ObjMetadataSet(setA).Equal(ObjMetadataSet(setB))
}

// Equal returns true if the two sets contain equivalent objects. Duplicates are
// ignored.
// This function satisfies the cmp.Equal interface from github.com/google/go-cmp
func (setA ObjMetadataSet) Equal(setB ObjMetadataSet) bool {
	mapA := make(map[ObjMetadata]struct{}, len(setA))
	for _, a := range setA {
		mapA[a] = struct{}{}
	}
	mapB := make(map[ObjMetadata]struct{}, len(setB))
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

// Contains checks if the provided ObjMetadata exists in the set.
func (setA ObjMetadataSet) Contains(id ObjMetadata) bool {
	for _, om := range setA {
		if om == id {
			return true
		}
	}
	return false
}

// Remove the object from the set and return the updated set.
func (setA ObjMetadataSet) Remove(obj ObjMetadata) ObjMetadataSet {
	for i, a := range setA {
		if a == obj {
			setA[len(setA)-1], setA[i] = setA[i], setA[len(setA)-1]
			return setA[:len(setA)-1]
		}
	}
	return setA
}

// Union returns the set of unique objects from the merging of set A and set B.
func (setA ObjMetadataSet) Union(setB ObjMetadataSet) ObjMetadataSet {
	m := make(map[ObjMetadata]struct{}, len(setA)+len(setB))
	for _, a := range setA {
		m[a] = struct{}{}
	}
	for _, b := range setB {
		m[b] = struct{}{}
	}
	union := make(ObjMetadataSet, 0, len(m))
	for u := range m {
		union = append(union, u)
	}
	return union
}

// Diff returns the set of objects that exist in set A, but not in set B (A - B).
func (setA ObjMetadataSet) Diff(setB ObjMetadataSet) ObjMetadataSet {
	// Create a map of the elements of A
	m := make(map[ObjMetadata]struct{}, len(setA))
	for _, a := range setA {
		m[a] = struct{}{}
	}
	// Remove from A each element of B
	for _, b := range setB {
		delete(m, b) // OK to delete even if b not in m
	}
	// Create/return slice from the map of remaining items
	diff := make(ObjMetadataSet, 0, len(m))
	for r := range m {
		diff = append(diff, r)
	}
	return diff
}

// Hash the objects in the set by serializing, sorting, concatonating, and
// hashing the result with the 32-bit FNV-1a algorithm.
func (setA ObjMetadataSet) Hash() (string, error) {
	objStrs := make([]string, 0, len(setA))
	for _, obj := range setA {
		objStrs = append(objStrs, obj.String())
	}
	hashInt, err := calcHash(objStrs)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(hashInt), 16), nil
}

// calcHash returns an unsigned int32 representing the hash
// of the obj metadata strings. If there is an error writing bytes to
// the hash, then the error is returned; nil is returned otherwise.
// Used to quickly identify the set of resources in the inventory object.
func calcHash(objs []string) (uint32, error) {
	sort.Strings(objs)
	h := fnv.New32a()
	for _, obj := range objs {
		_, err := h.Write([]byte(obj))
		if err != nil {
			return uint32(0), err
		}
	}
	return h.Sum32(), nil
}

// ToStringMap returns the set as a serializable map, with objMeta keys and
// empty string values.
func (setA ObjMetadataSet) ToStringMap() map[string]string {
	stringMap := make(map[string]string, len(setA))
	for _, objMeta := range setA {
		stringMap[objMeta.String()] = ""
	}
	return stringMap
}

// FromStringMap returns a set from a serializable map, with objMeta keys and
// empty string values. Errors if parsing fails.
func FromStringMap(in map[string]string) (ObjMetadataSet, error) {
	var set ObjMetadataSet
	for s := range in {
		objMeta, err := ParseObjMetadata(s)
		if err != nil {
			return nil, err
		}
		set = append(set, objMeta)
	}
	return set, nil
}

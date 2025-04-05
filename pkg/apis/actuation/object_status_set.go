// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package actuation

// ObjectStatusSet is an ordered list of ObjectStatus that acts like an
// unordered set for comparison purposes.
type ObjectStatusSet []ObjectStatus

// Equal returns true if the two sets contain equivalent objects. Duplicates are
// ignored.
// This function satisfies the cmp.Equal interface from github.com/google/go-cmp
func (setA ObjectStatusSet) Equal(setB ObjectStatusSet) bool {
	mapA := make(map[ObjectStatus]struct{}, len(setA))
	for _, a := range setA {
		mapA[a] = struct{}{}
	}
	mapB := make(map[ObjectStatus]struct{}, len(setB))
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

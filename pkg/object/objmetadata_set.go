// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//

package object

// UnstructuredSetEquals returns true if the slice of objects in setA equals
// the slice of objects in setB.
func ObjMetadataSetEquals(setA []ObjMetadata, setB []ObjMetadata) bool {
	return ObjMetadataSet(setA).Equal(ObjMetadataSet(setB))
}

type ObjMetadataSet []ObjMetadata

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

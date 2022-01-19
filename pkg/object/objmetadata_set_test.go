// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var objMeta1 = ObjMetadata{
	GroupKind: schema.GroupKind{
		Group: "apps",
		Kind:  "Deployment",
	},
	Name:      "dep",
	Namespace: "default",
}

var objMeta2 = ObjMetadata{
	GroupKind: schema.GroupKind{
		Group: "apps",
		Kind:  "StatefulSet",
	},
	Name:      "dep",
	Namespace: "default",
}

var objMeta3 = ObjMetadata{
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Pod",
	},
	Name:      "pod-a",
	Namespace: "default",
}

var objMeta4 = ObjMetadata{
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Pod",
	},
	Name:      "pod-b",
	Namespace: "default",
}

func TestObjMetadataSetEquals(t *testing.T) {
	testCases := map[string]struct {
		setA    ObjMetadataSet
		setB    ObjMetadataSet
		isEqual bool
	}{
		"Empty sets results in empty union": {
			setA:    ObjMetadataSet{},
			setB:    ObjMetadataSet{},
			isEqual: true,
		},
		"Empty second set results in same set": {
			setA:    ObjMetadataSet{objMeta1, objMeta3},
			setB:    ObjMetadataSet{},
			isEqual: false,
		},
		"Empty initial set results in empty diff": {
			setA:    ObjMetadataSet{},
			setB:    ObjMetadataSet{objMeta1, objMeta3},
			isEqual: false,
		},
		"Different ordering are equal sets": {
			setA:    ObjMetadataSet{objMeta2, objMeta1},
			setB:    ObjMetadataSet{objMeta1, objMeta2},
			isEqual: true,
		},
		"One item overlap": {
			setA:    ObjMetadataSet{objMeta2, objMeta1},
			setB:    ObjMetadataSet{objMeta1, objMeta3},
			isEqual: false,
		},
		"Disjoint sets results in larger set": {
			setA:    ObjMetadataSet{objMeta1, objMeta2},
			setB:    ObjMetadataSet{objMeta3, objMeta4},
			isEqual: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := tc.setA.Equal(tc.setB)
			if tc.isEqual != actual {
				t.Errorf("Equal expected (%t), got (%t)", tc.isEqual, actual)
			}
		})
	}
}

func TestObjMetadataSetUnion(t *testing.T) {
	testCases := map[string]struct {
		setA     ObjMetadataSet
		setB     ObjMetadataSet
		expected ObjMetadataSet
	}{
		"Empty sets results in empty union": {
			setA:     ObjMetadataSet{},
			setB:     ObjMetadataSet{},
			expected: ObjMetadataSet{},
		},
		"Empty second set results in same set": {
			setA:     ObjMetadataSet{objMeta1, objMeta3},
			setB:     ObjMetadataSet{},
			expected: ObjMetadataSet{objMeta1, objMeta3},
		},
		"Empty initial set results in empty diff": {
			setA:     ObjMetadataSet{},
			setB:     ObjMetadataSet{objMeta1, objMeta3},
			expected: ObjMetadataSet{objMeta1, objMeta3},
		},
		"Same sets in different order results in same set": {
			setA:     ObjMetadataSet{objMeta2, objMeta1},
			setB:     ObjMetadataSet{objMeta1, objMeta2},
			expected: ObjMetadataSet{objMeta1, objMeta2},
		},
		"One item overlap": {
			setA:     ObjMetadataSet{objMeta2, objMeta1},
			setB:     ObjMetadataSet{objMeta1, objMeta3},
			expected: ObjMetadataSet{objMeta1, objMeta2, objMeta3},
		},
		"Disjoint sets results in larger set": {
			setA:     ObjMetadataSet{objMeta1, objMeta2},
			setB:     ObjMetadataSet{objMeta3, objMeta4},
			expected: ObjMetadataSet{objMeta1, objMeta2, objMeta3, objMeta4},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := tc.setA.Union(tc.setB)
			if !tc.expected.Equal(actual) {
				t.Errorf("Union expected set (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestObjMetadataSetDiff(t *testing.T) {
	testCases := map[string]struct {
		setA     ObjMetadataSet
		setB     ObjMetadataSet
		expected ObjMetadataSet
	}{
		"Empty sets results in empty diff": {
			setA:     ObjMetadataSet{},
			setB:     ObjMetadataSet{},
			expected: ObjMetadataSet{},
		},
		"Empty subtraction set results in same set": {
			setA:     ObjMetadataSet{objMeta1, objMeta3},
			setB:     ObjMetadataSet{},
			expected: ObjMetadataSet{objMeta1, objMeta3},
		},
		"Empty initial set results in empty diff": {
			setA:     ObjMetadataSet{},
			setB:     ObjMetadataSet{objMeta1, objMeta3},
			expected: ObjMetadataSet{},
		},
		"Sets equal results in empty diff": {
			setA:     ObjMetadataSet{objMeta2, objMeta1},
			setB:     ObjMetadataSet{objMeta1, objMeta2},
			expected: ObjMetadataSet{},
		},
		"Basic diff": {
			setA:     ObjMetadataSet{objMeta2, objMeta1},
			setB:     ObjMetadataSet{objMeta1},
			expected: ObjMetadataSet{objMeta2},
		},
		"Subtract non-elements results in no change": {
			setA:     ObjMetadataSet{objMeta1},
			setB:     ObjMetadataSet{objMeta3, objMeta4},
			expected: ObjMetadataSet{objMeta1},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := tc.setA.Diff(tc.setB)
			if !tc.expected.Equal(actual) {
				t.Errorf("Diff expected set (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestObjMetadataSetHash(t *testing.T) {
	tests := map[string]struct {
		objs     ObjMetadataSet
		expected string
	}{
		"No objects gives valid hash": {
			objs:     ObjMetadataSet{},
			expected: "811c9dc5",
		},
		"Single object gives valid hash": {
			objs:     ObjMetadataSet{objMeta1},
			expected: "3715cd95",
		},
		"Multiple objects gives valid hash": {
			objs:     ObjMetadataSet{objMeta1, objMeta2, objMeta3},
			expected: "d69d726a",
		},
		"Different ordering gives same hash": {
			objs:     ObjMetadataSet{objMeta2, objMeta3, objMeta1},
			expected: "d69d726a",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := tc.objs.Hash()
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCreateObjMetadata(t *testing.T) {
	tests := []struct {
		namespace string
		name      string
		gk        schema.GroupKind
		expected  string
		isError   bool
	}{
		{
			namespace: "  \n",
			name:      " test-name\t",
			gk: schema.GroupKind{
				Group: "apps",
				Kind:  "ReplicaSet",
			},
			expected: "_test-name_apps_ReplicaSet",
			isError:  false,
		},
		{
			namespace: "test-namespace ",
			name:      " test-name\t",
			gk: schema.GroupKind{
				Group: "apps",
				Kind:  "ReplicaSet",
			},
			expected: "test-namespace_test-name_apps_ReplicaSet",
			isError:  false,
		},
		// Error with empty name.
		{
			namespace: "test-namespace ",
			name:      " \t",
			gk: schema.GroupKind{
				Group: "apps",
				Kind:  "ReplicaSet",
			},
			expected: "",
			isError:  true,
		},
		// Error with empty GroupKind.
		{
			namespace: "test-namespace",
			name:      "test-name",
			gk:        schema.GroupKind{},
			expected:  "",
			isError:   true,
		},
		// Error with invalid name characters "_".
		{
			namespace: "test-namespace",
			name:      "test_name", // Invalid "_" character
			gk: schema.GroupKind{
				Group: "apps",
				Kind:  "ReplicaSet",
			},
			expected: "",
			isError:  true,
		},
		// Error name not starting with alphanumeric char
		{
			namespace: "test-namespace",
			name:      "-test",
			gk: schema.GroupKind{
				Group: "apps",
				Kind:  "ReplicaSet",
			},
			expected: "",
			isError:  true,
		},
		// Error name not ending with alphanumeric char
		{
			namespace: "test-namespace",
			name:      "test-",
			gk: schema.GroupKind{
				Group: "apps",
				Kind:  "ReplicaSet",
			},
			expected: "",
			isError:  true,
		},
	}

	for _, test := range tests {
		inv, err := CreateObjMetadata(test.namespace, test.name, test.gk)
		if !test.isError {
			if err != nil {
				t.Errorf("Error creating ObjMetadata when it should have worked.")
			} else if test.expected != inv.String() {
				t.Errorf("Expected inventory\n(%s) != created inventory\n(%s)\n", test.expected, inv.String())
			}
			// Parsing back the just created inventory string to ObjMetadata,
			// so that tests will catch any change to CreateObjMetadata that
			// would break ParseObjMetadata.
			expectedObjMetadata := &ObjMetadata{
				Namespace: strings.TrimSpace(test.namespace),
				Name:      strings.TrimSpace(test.name),
				GroupKind: test.gk,
			}
			actual, err := ParseObjMetadata(inv.String())
			if err != nil {
				t.Errorf("Error parsing back ObjMetadata, when it should have worked.")
			} else if !expectedObjMetadata.Equals(&actual) {
				t.Errorf("Expected inventory (%s) != parsed inventory (%s)\n", expectedObjMetadata, actual)
			}
		}
		if test.isError && err == nil {
			t.Errorf("Should have returned an error in CreateObjMetadata()")
		}
	}
}

func TestObjMetadataEquals(t *testing.T) {
	testCases := map[string]struct {
		objMeta1     *ObjMetadata
		objMeta2     *ObjMetadata
		expectEquals bool
	}{
		"parameter is nil": {
			objMeta1: &ObjMetadata{
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "Deployment",
				},
				Name:      "dep",
				Namespace: "default",
			},
			objMeta2:     nil,
			expectEquals: false,
		},
		"different groupKind": {
			objMeta1: &ObjMetadata{
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "StatefulSet",
				},
				Name:      "dep",
				Namespace: "default",
			},
			objMeta2: &ObjMetadata{
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "Deployment",
				},
				Name:      "dep",
				Namespace: "default",
			},
			expectEquals: false,
		},
		"both are missing groupKind": {
			objMeta1: &ObjMetadata{
				Name:      "dep",
				Namespace: "default",
			},
			objMeta2: &ObjMetadata{
				Name:      "dep",
				Namespace: "default",
			},
			expectEquals: true,
		},
		"they are equal": {
			objMeta1: &ObjMetadata{
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "Deployment",
				},
				Name:      "dep",
				Namespace: "default",
			},
			objMeta2: &ObjMetadata{
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "Deployment",
				},
				Name:      "dep",
				Namespace: "default",
			},
			expectEquals: true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			equal := tc.objMeta1.Equals(tc.objMeta2)

			if tc.expectEquals && !equal {
				t.Error("Expected objMetas to be equal, but they weren't")
			}

			if !tc.expectEquals && equal {
				t.Error("Expected objMetas not to be equal, but they were")
			}
		})
	}
}

func TestParseObjMetadata(t *testing.T) {
	tests := []struct {
		invStr    string
		inventory *ObjMetadata
		isError   bool
	}{
		{
			invStr: "_test-name_apps_ReplicaSet\t",
			inventory: &ObjMetadata{
				Name: "test-name",
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "ReplicaSet",
				},
			},
			isError: false,
		},
		{
			invStr: "test-namespace_test-name_apps_Deployment",
			inventory: &ObjMetadata{
				Namespace: "test-namespace",
				Name:      "test-name",
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "Deployment",
				},
			},
			isError: false,
		},
		// Not enough fields -- error
		{
			invStr:    "_test-name_apps",
			inventory: &ObjMetadata{},
			isError:   true,
		},
		// Too many fields
		{
			invStr:    "test-namespace_test-name_apps_foo_Deployment",
			inventory: &ObjMetadata{},
			isError:   true,
		},
	}

	for _, test := range tests {
		actual, err := ParseObjMetadata(test.invStr)
		if !test.isError {
			if err != nil {
				t.Errorf("Error parsing inventory when it should have worked.")
			} else if !test.inventory.Equals(&actual) {
				t.Errorf("Expected inventory (%s) != parsed inventory (%s)\n", test.inventory, actual)
			}
		}
		if test.isError && err == nil {
			t.Errorf("Should have returned an error in ParseObjMetadata()")
		}
	}
}

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

func TestHash(t *testing.T) {
	tests := map[string]struct {
		objs     []ObjMetadata
		expected string
	}{
		"No objects gives valid hash": {
			objs:     []ObjMetadata{},
			expected: "811c9dc5",
		},
		"Single object gives valid hash": {
			objs:     []ObjMetadata{objMeta1},
			expected: "3715cd95",
		},
		"Multiple objects gives valid hash": {
			objs:     []ObjMetadata{objMeta1, objMeta2, objMeta3},
			expected: "d69d726a",
		},
		"Different ordering gives same hash": {
			objs:     []ObjMetadata{objMeta2, objMeta3, objMeta1},
			expected: "d69d726a",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := Hash(tc.objs)
			if err != nil {
				t.Fatalf("Received unexpected error: %s", err)
			}
			if tc.expected != actual {
				t.Errorf("expected hash string (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestSetDiff(t *testing.T) {
	testCases := map[string]struct {
		setA     []ObjMetadata
		setB     []ObjMetadata
		expected []ObjMetadata
	}{
		"Empty sets results in empty diff": {
			setA:     []ObjMetadata{},
			setB:     []ObjMetadata{},
			expected: []ObjMetadata{},
		},
		"Empty subtraction set results in same set": {
			setA:     []ObjMetadata{objMeta1, objMeta3},
			setB:     []ObjMetadata{},
			expected: []ObjMetadata{objMeta1, objMeta3},
		},
		"Empty initial set results in empty diff": {
			setA:     []ObjMetadata{},
			setB:     []ObjMetadata{objMeta1, objMeta3},
			expected: []ObjMetadata{},
		},
		"Sets equal results in empty diff": {
			setA:     []ObjMetadata{objMeta2, objMeta1},
			setB:     []ObjMetadata{objMeta1, objMeta2},
			expected: []ObjMetadata{},
		},
		"Basic diff": {
			setA:     []ObjMetadata{objMeta2, objMeta1},
			setB:     []ObjMetadata{objMeta1},
			expected: []ObjMetadata{objMeta2},
		},
		"Subtract non-elements results in no change": {
			setA:     []ObjMetadata{objMeta1},
			setB:     []ObjMetadata{objMeta3, objMeta4},
			expected: []ObjMetadata{objMeta1},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := SetDiff(tc.setA, tc.setB)
			if !SetEquals(tc.expected, actual) {
				t.Errorf("SetDiff expected set (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestUnion(t *testing.T) {
	testCases := map[string]struct {
		setA     []ObjMetadata
		setB     []ObjMetadata
		expected []ObjMetadata
	}{
		"Empty sets results in empty union": {
			setA:     []ObjMetadata{},
			setB:     []ObjMetadata{},
			expected: []ObjMetadata{},
		},
		"Empty second set results in same set": {
			setA:     []ObjMetadata{objMeta1, objMeta3},
			setB:     []ObjMetadata{},
			expected: []ObjMetadata{objMeta1, objMeta3},
		},
		"Empty initial set results in empty diff": {
			setA:     []ObjMetadata{},
			setB:     []ObjMetadata{objMeta1, objMeta3},
			expected: []ObjMetadata{objMeta1, objMeta3},
		},
		"Same sets in different order results in same set": {
			setA:     []ObjMetadata{objMeta2, objMeta1},
			setB:     []ObjMetadata{objMeta1, objMeta2},
			expected: []ObjMetadata{objMeta1, objMeta2},
		},
		"One item overlap": {
			setA:     []ObjMetadata{objMeta2, objMeta1},
			setB:     []ObjMetadata{objMeta1, objMeta3},
			expected: []ObjMetadata{objMeta1, objMeta2, objMeta3},
		},
		"Disjoint sets results in larger set": {
			setA:     []ObjMetadata{objMeta1, objMeta2},
			setB:     []ObjMetadata{objMeta3, objMeta4},
			expected: []ObjMetadata{objMeta1, objMeta2, objMeta3, objMeta4},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := Union(tc.setA, tc.setB)
			if !SetEquals(tc.expected, actual) {
				t.Errorf("SetDiff expected set (%s), got (%s)", tc.expected, actual)
			}
		})
	}
}

func TestSetEquals(t *testing.T) {
	testCases := map[string]struct {
		setA    []ObjMetadata
		setB    []ObjMetadata
		isEqual bool
	}{
		"Empty sets results in empty union": {
			setA:    []ObjMetadata{},
			setB:    []ObjMetadata{},
			isEqual: true,
		},
		"Empty second set results in same set": {
			setA:    []ObjMetadata{objMeta1, objMeta3},
			setB:    []ObjMetadata{},
			isEqual: false,
		},
		"Empty initial set results in empty diff": {
			setA:    []ObjMetadata{},
			setB:    []ObjMetadata{objMeta1, objMeta3},
			isEqual: false,
		},
		"Different ordering are equal sets": {
			setA:    []ObjMetadata{objMeta2, objMeta1},
			setB:    []ObjMetadata{objMeta1, objMeta2},
			isEqual: true,
		},
		"One item overlap": {
			setA:    []ObjMetadata{objMeta2, objMeta1},
			setB:    []ObjMetadata{objMeta1, objMeta3},
			isEqual: false,
		},
		"Disjoint sets results in larger set": {
			setA:    []ObjMetadata{objMeta1, objMeta2},
			setB:    []ObjMetadata{objMeta3, objMeta4},
			isEqual: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := SetEquals(tc.setA, tc.setB)
			if tc.isEqual != actual {
				t.Errorf("SetEqual expected (%t), got (%t)", tc.isEqual, actual)
			}
		})
	}
}

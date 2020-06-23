// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
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
		{
			name: "psp:rook",
			gk: schema.GroupKind{
				Group: "rbac.authorization.k8s.io",
				Kind:  "ClusterRole",
			},
			expected: "_psp_rook_rbac.authorization.k8s.io_ClusterRole",
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
	}

	for _, test := range tests {
		inv, err := CreateObjMetadata(test.namespace, test.name, test.gk)
		if !test.isError {
			if err != nil {
				t.Errorf("Error creating inventory when it should have worked.")
			} else if test.expected != inv.String() {
				t.Errorf("Expected inventory (%s) != created inventory(%s)\n", test.expected, inv.String())
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
	}

	for _, test := range tests {
		actual, err := ParseObjMetadata(test.invStr)
		if !test.isError {
			if err != nil {
				t.Errorf("Error parsing inventory when it should have worked.")
			} else if !test.inventory.Equals(actual) {
				t.Errorf("Expected inventory (%s) != parsed inventory (%s)\n", test.inventory, actual)
			}
		}
		if test.isError && err == nil {
			t.Errorf("Should have returned an error in ParseObjMetadata()")
		}
	}
}

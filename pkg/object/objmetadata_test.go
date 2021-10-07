// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCreateObjMetadata(t *testing.T) {
	tests := map[string]struct {
		namespace string
		name      string
		gk        schema.GroupKind
		expected  string
		isError   bool
	}{
		"Empty name is an error": {
			namespace: "test-namespace ",
			name:      "",
			gk: schema.GroupKind{
				Group: "apps",
				Kind:  "ReplicaSet",
			},
			expected: "",
			isError:  true,
		},
		"Empty GroupKind is an error": {
			namespace: "test-namespace",
			name:      "test-name",
			gk:        schema.GroupKind{},
			expected:  "",
			isError:   true,
		},
		"Colon is allowed in the name for RBAC resources": {
			namespace: "test-namespace",
			name:      "system::kube-scheduler",
			gk: schema.GroupKind{
				Group: rbacv1.GroupName,
				Kind:  "Role",
			},
			expected: "test-namespace_system____kube-scheduler_rbac.authorization.k8s.io_Role",
			isError:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			inv, err := CreateObjMetadata(tc.namespace, tc.name, tc.gk)
			if !tc.isError {
				if err != nil {
					t.Errorf("Error creating ObjMetadata when it should have worked.")
				} else if tc.expected != inv.String() {
					t.Errorf("Expected inventory\n(%s) != created inventory\n(%s)\n", tc.expected, inv.String())
				}
				// Parsing back the just created inventory string to ObjMetadata,
				// so that tests will catch any change to CreateObjMetadata that
				// would break ParseObjMetadata.
				expectedObjMetadata := &ObjMetadata{
					Namespace: tc.namespace,
					Name:      tc.name,
					GroupKind: tc.gk,
				}
				actual, err := ParseObjMetadata(inv.String())
				if err != nil {
					t.Errorf("Error parsing back ObjMetadata, when it should have worked.")
				} else if !expectedObjMetadata.Equals(&actual) {
					t.Errorf("Expected inventory (%s) != parsed inventory (%s)\n", expectedObjMetadata, actual)
				}
			}
			if tc.isError && err == nil {
				t.Errorf("Should have returned an error in CreateObjMetadata()")
			}
		})
	}
}

func TestParseObjMetadata(t *testing.T) {
	tests := map[string]struct {
		invStr    string
		inventory *ObjMetadata
		isError   bool
	}{
		"Simple inventory string parse with empty namespace": {
			invStr: "_test-name_apps_ReplicaSet",
			inventory: &ObjMetadata{
				Name: "test-name",
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "ReplicaSet",
				},
			},
			isError: false,
		},
		"Basic inventory string parse": {
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
		"RBAC resources can have colon (double underscore) in their name": {
			invStr: "test-namespace_kubeadm__nodes-kubeadm-config_rbac.authorization.k8s.io_Role",
			inventory: &ObjMetadata{
				Namespace: "test-namespace",
				Name:      "kubeadm:nodes-kubeadm-config",
				GroupKind: schema.GroupKind{
					Group: rbacv1.GroupName,
					Kind:  "Role",
				},
			},
			isError: false,
		},
		"RBAC resources can have double colon (double underscore) in their name": {
			invStr: "test-namespace_system____leader-locking-kube-scheduler_rbac.authorization.k8s.io_Role",
			inventory: &ObjMetadata{
				Namespace: "test-namespace",
				Name:      "system::leader-locking-kube-scheduler",
				GroupKind: schema.GroupKind{
					Group: rbacv1.GroupName,
					Kind:  "Role",
				},
			},
			isError: false,
		},
		"Test double underscore (colon) at beginning of name": {
			invStr: "test-namespace___leader-locking-kube-scheduler_rbac.authorization.k8s.io_ClusterRole",
			inventory: &ObjMetadata{
				Namespace: "test-namespace",
				Name:      ":leader-locking-kube-scheduler",
				GroupKind: schema.GroupKind{
					Group: rbacv1.GroupName,
					Kind:  "ClusterRole",
				},
			},
			isError: false,
		},
		"Test double underscore (colon) at end of name": {
			invStr: "test-namespace_leader-locking-kube-scheduler___rbac.authorization.k8s.io_RoleBinding",
			inventory: &ObjMetadata{
				Namespace: "test-namespace",
				Name:      "leader-locking-kube-scheduler:",
				GroupKind: schema.GroupKind{
					Group: rbacv1.GroupName,
					Kind:  "RoleBinding",
				},
			},
			isError: false,
		},
		"Not enough fields -- error": {
			invStr:    "_test-name_apps",
			inventory: &ObjMetadata{},
			isError:   true,
		},
		"Only one field (no separators) -- error": {
			invStr:    "test-namespacetest-nametest-grouptest-kind",
			inventory: &ObjMetadata{},
			isError:   true,
		},
		"Too many fields": {
			invStr:    "test-namespace_test-name_apps_foo_Deployment",
			inventory: &ObjMetadata{},
			isError:   true,
		},
	}

	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			actual, err := ParseObjMetadata(tc.invStr)
			if !tc.isError {
				if err != nil {
					t.Errorf("Error parsing inventory when it should have worked: %s", err)
				} else if !tc.inventory.Equals(&actual) {
					t.Errorf("Expected inventory (%s) != parsed inventory (%s)\n", tc.inventory, actual)
				}
			}
			if tc.isError && err == nil {
				t.Errorf("Should have returned an error in ParseObjMetadata()")
			}
		})
	}
}

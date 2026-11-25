// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
		"Cluster-scoped RBAC resource with a literal underscore in its name": {
			invStr: "_roles_ops_rbac.authorization.k8s.io_ClusterRole",
			inventory: &ObjMetadata{
				Name: "roles_ops",
				GroupKind: schema.GroupKind{
					Group: rbacv1.GroupName,
					Kind:  "ClusterRole",
				},
			},
			isError: false,
		},
		"RBAC resource with an ambiguous run of underscores -- error": {
			invStr:    "_a___b_rbac.authorization.k8s.io_ClusterRole",
			inventory: &ObjMetadata{},
			isError:   true,
		},
		"Non-RBAC resource with an underscore in its name -- error": {
			invStr:    "test-namespace_test_name_apps_Deployment",
			inventory: &ObjMetadata{},
			isError:   true,
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

func TestObjMetadataRBACNameRoundTrip(t *testing.T) {
	clusterRole := schema.GroupKind{Group: rbacv1.GroupName, Kind: "ClusterRole"}
	role := schema.GroupKind{Group: rbacv1.GroupName, Kind: "Role"}
	tests := map[string]struct {
		id      ObjMetadata
		isError bool
	}{
		"underscore in the middle":          {id: ObjMetadata{Name: "roles_ops", GroupKind: clusterRole}},
		"leading underscore":                {id: ObjMetadata{Name: "_ops", GroupKind: clusterRole}},
		"trailing underscore":               {id: ObjMetadata{Name: "ops_", GroupKind: clusterRole}},
		"namespaced Role with underscore":   {id: ObjMetadata{Namespace: "test-namespace", Name: "roles_ops", GroupKind: role}},
		"underscore and colon not adjacent": {id: ObjMetadata{Name: "system:roles_ops", GroupKind: clusterRole}},
		"several separate underscores":      {id: ObjMetadata{Name: "a_b_c", GroupKind: clusterRole}},
		"double colon":                      {id: ObjMetadata{Name: "a::b", GroupKind: clusterRole}},
		// "a:_b", "a_:b" and "a___b" all encode to "a___b", so parsing must
		// fail instead of guessing which object is meant.
		"colon followed by underscore -- error": {id: ObjMetadata{Name: "a:_b", GroupKind: clusterRole}, isError: true},
		"underscore followed by colon -- error": {id: ObjMetadata{Name: "a_:b", GroupKind: clusterRole}, isError: true},
		"three underscores -- error":            {id: ObjMetadata{Name: "a___b", GroupKind: clusterRole}, isError: true},
	}
	for tn, tc := range tests {
		t.Run(tn, func(t *testing.T) {
			s := tc.id.String()
			actual, err := ParseObjMetadata(s)
			if tc.isError {
				if err == nil {
					t.Errorf("expected an error parsing %q, got %v", s, actual)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error parsing %q: %v", s, err)
			}
			if actual != tc.id {
				t.Errorf("round trip of %q: expected %v, got %v", s, tc.id, actual)
			}
		})
	}
}

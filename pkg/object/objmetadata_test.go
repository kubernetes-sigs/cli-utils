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
		"RBAC with underscores": {
			invStr: "test-namespace_leader_locking_kube_scheduler___rbac.authorization.k8s.io_RoleBinding",
			inventory: &ObjMetadata{
				Namespace: "test-namespace",
				Name:      "leader_locking_kube_scheduler:",
				GroupKind: schema.GroupKind{
					Group: rbacv1.GroupName,
					Kind:  "RoleBinding",
				},
			},
			isError: false,
		},
		"Non-RBAC with underscores": {
			invStr: "test-namespace_leader_locking_kube_scheduler_apps_Deployment",
			inventory: &ObjMetadata{
				Namespace: "test-namespace",
				Name:      "leader_locking_kube_scheduler",
				GroupKind: schema.GroupKind{
					Group: "apps",
					Kind:  "Deployment",
				},
			},
			isError: true,
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

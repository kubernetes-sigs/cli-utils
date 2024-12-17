// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestBuildObjMap(t *testing.T) {
	obj1 := actuation.ObjectReference{
		Group:     "group1",
		Kind:      "Kind",
		Namespace: "ns",
		Name:      "na",
	}
	obj2 := actuation.ObjectReference{
		Group:     "group2",
		Kind:      "Kind",
		Namespace: "ns",
		Name:      "na",
	}

	tests := map[string]struct {
		objSet    object.ObjMetadataSet
		objStatus []actuation.ObjectStatus
		expected  map[string]string
		hasError  bool
	}{
		"objMetadata matches the status": {
			objSet: object.ObjMetadataSet{ObjMetadataFromObjectReference(obj1), ObjMetadataFromObjectReference(obj2)},
			objStatus: []actuation.ObjectStatus{
				{
					ObjectReference: obj1,
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcilePending,
				},
				{
					ObjectReference: obj2,
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSkipped,
					Reconcile:       actuation.ReconcileSucceeded,
				},
			},
			expected: map[string]string{
				"ns_na_group1_Kind": `{"actuation":"Succeeded","reconcile":"Pending","strategy":"Apply"}`,
				"ns_na_group2_Kind": `{"actuation":"Skipped","reconcile":"Succeeded","strategy":"Delete"}`,
			},
		},
		"empty object status list": {
			objSet:   object.ObjMetadataSet{ObjMetadataFromObjectReference(obj1), ObjMetadataFromObjectReference(obj2)},
			hasError: false,
			expected: map[string]string{
				"ns_na_group1_Kind": "",
				"ns_na_group2_Kind": "",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := buildObjMap(tc.objSet, tc.objStatus)
			if diff := cmp.Diff(actual, tc.expected); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
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
		objSet        object.ObjMetadataSet
		objStatus     object.ObjectStatusSet
		statusEnabled bool
		expected      map[string]string
		hasError      bool
	}{
		"status included and enabled": {
			objSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
			objStatus: object.ObjectStatusSet{
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
			statusEnabled: true,
			expected: map[string]string{
				"ns_na_group1_Kind": `{"strategy":"Apply","actuation":"Succeeded","reconcile":"Pending"}`,
				"ns_na_group2_Kind": `{"strategy":"Delete","actuation":"Skipped","reconcile":"Succeeded"}`,
			},
		},
		"status included and disabled": {
			objSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
			objStatus: object.ObjectStatusSet{
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
			statusEnabled: false,
			expected: map[string]string{
				"ns_na_group1_Kind": "",
				"ns_na_group2_Kind": "",
			},
		},
		"status empty and enabled": {
			objSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
			statusEnabled: true,
			hasError:      false,
			expected: map[string]string{
				"ns_na_group1_Kind": "",
				"ns_na_group2_Kind": "",
			},
		},
		"status empty and disabled": {
			objSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
			statusEnabled: false,
			hasError:      false,
			expected: map[string]string{
				"ns_na_group1_Kind": "",
				"ns_na_group2_Kind": "",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := buildDataMap(tc.objSet, tc.objStatus, tc.statusEnabled)
			if tc.hasError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			testutil.AssertEqual(t, tc.expected, actual)
		})
	}
}

func TestParseObjMap(t *testing.T) {
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
		dataMap              map[string]string
		statusEnabled        bool
		expectedObjRefSet    object.ObjMetadataSet
		expectedObjStatusSet object.ObjectStatusSet
		hasError             bool
	}{
		"status included and enabled": {
			dataMap: map[string]string{
				"ns_na_group1_Kind": `{"strategy":"Apply","actuation":"Succeeded","reconcile":"Pending"}`,
				"ns_na_group2_Kind": `{"strategy":"Delete","actuation":"Skipped","reconcile":"Succeeded"}`,
			},
			statusEnabled: true,
			expectedObjRefSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
			expectedObjStatusSet: object.ObjectStatusSet{
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
		},
		"status included and disabled": {
			dataMap: map[string]string{
				"ns_na_group1_Kind": `{"strategy":"Apply","actuation":"Succeeded","reconcile":"Pending"}`,
				"ns_na_group2_Kind": `{"strategy":"Delete","actuation":"Skipped","reconcile":"Succeeded"}`,
			},
			statusEnabled: false,
			expectedObjRefSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
		},
		"status empty and enabled": {
			dataMap: map[string]string{
				"ns_na_group1_Kind": "",
				"ns_na_group2_Kind": "",
			},
			expectedObjRefSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
			expectedObjStatusSet: object.ObjectStatusSet{
				{
					ObjectReference: obj1,
				},
				{
					ObjectReference: obj2,
				},
			},
			statusEnabled: true,
			hasError:      false,
		},
		"status empty and disabled": {
			dataMap: map[string]string{
				"ns_na_group1_Kind": "",
				"ns_na_group2_Kind": "",
			},
			expectedObjRefSet: object.ObjMetadataSet{
				ObjMetadataFromObjectReference(obj1),
				ObjMetadataFromObjectReference(obj2),
			},
			statusEnabled: false,
			hasError:      false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualObjRefSet, actualObjStatusSet, err := parseDataMap(tc.dataMap, tc.statusEnabled)
			if tc.hasError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			testutil.AssertEqual(t, tc.expectedObjRefSet, actualObjRefSet)
			testutil.AssertEqual(t, tc.expectedObjStatusSet, actualObjStatusSet)
		})
	}
}

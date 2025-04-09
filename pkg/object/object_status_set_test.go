// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
)

var objStatus1 = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     "apps",
		Kind:      "Deployment",
		Name:      "dep",
		Namespace: "default",
	},
	Strategy:   actuation.ActuationStrategyApply,
	Actuation:  actuation.ActuationSucceeded,
	Reconcile:  actuation.ReconcileSucceeded,
	UID:        "123",
	Generation: 456,
}

var objStatus2 = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     "apps",
		Kind:      "StatefulSet",
		Name:      "dep",
		Namespace: "default",
	},
	Strategy:   actuation.ActuationStrategyApply,
	Actuation:  actuation.ActuationSucceeded,
	Reconcile:  actuation.ReconcileSucceeded,
	UID:        "234",
	Generation: 567,
}

var objStatus3 = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     "",
		Kind:      "Pod",
		Name:      "pod-a",
		Namespace: "default",
	},
	Strategy:   actuation.ActuationStrategyApply,
	Actuation:  actuation.ActuationSucceeded,
	Reconcile:  actuation.ReconcileSucceeded,
	UID:        "345",
	Generation: 678,
}

var objStatus4 = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     "",
		Kind:      "Pod",
		Name:      "pod-b",
		Namespace: "default",
	},
	Strategy:   actuation.ActuationStrategyApply,
	Actuation:  actuation.ActuationSucceeded,
	Reconcile:  actuation.ReconcileSucceeded,
	UID:        "456",
	Generation: 789,
}

var objStatus1DiffStrategy = actuation.ObjectStatus{
	ObjectReference: objStatus1.ObjectReference,
	Strategy:        actuation.ActuationStrategyDelete, // Different Strategy
	Actuation:       objStatus1.Actuation,
	Reconcile:       objStatus1.Reconcile,
	UID:             objStatus1.UID,
	Generation:      objStatus1.Generation,
}

var objStatus1DiffActuation = actuation.ObjectStatus{
	ObjectReference: objStatus1.ObjectReference,
	Strategy:        objStatus1.Strategy,
	Actuation:       actuation.ActuationFailed, // Different Actuation
	Reconcile:       objStatus1.Reconcile,
	UID:             objStatus1.UID,
	Generation:      objStatus1.Generation,
}

var objStatus1DiffReconcile = actuation.ObjectStatus{
	ObjectReference: objStatus1.ObjectReference,
	Strategy:        objStatus1.Strategy,
	Actuation:       objStatus1.Actuation,
	Reconcile:       actuation.ReconcileFailed, // Different Reconcile
	UID:             objStatus1.UID,
	Generation:      objStatus1.Generation,
}

var objStatus1DiffUID = actuation.ObjectStatus{
	ObjectReference: objStatus1.ObjectReference,
	Strategy:        objStatus1.Strategy,
	Actuation:       objStatus1.Actuation,
	Reconcile:       objStatus1.Reconcile,
	UID:             "123-changed", // Different UID
	Generation:      objStatus1.Generation,
}

var objStatus1DiffGeneration = actuation.ObjectStatus{
	ObjectReference: objStatus1.ObjectReference,
	Strategy:        objStatus1.Strategy,
	Actuation:       objStatus1.Actuation,
	Reconcile:       objStatus1.Reconcile,
	UID:             objStatus1.UID,
	Generation:      457, // Different Generation
}

var objStatus1DiffName = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     objStatus1.ObjectReference.Group,
		Kind:      objStatus1.ObjectReference.Kind,
		Name:      "dep-changed", // Different Name
		Namespace: objStatus1.ObjectReference.Namespace,
	},
	Strategy:   objStatus1.Strategy,
	Actuation:  objStatus1.Actuation,
	Reconcile:  objStatus1.Reconcile,
	UID:        objStatus1.UID,
	Generation: objStatus1.Generation,
}

var objStatus1DiffNamespace = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     objStatus1.ObjectReference.Group,
		Kind:      objStatus1.ObjectReference.Kind,
		Name:      objStatus1.ObjectReference.Name,
		Namespace: "other", // Different Namespace
	},
	Strategy:   objStatus1.Strategy,
	Actuation:  objStatus1.Actuation,
	Reconcile:  objStatus1.Reconcile,
	UID:        objStatus1.UID,
	Generation: objStatus1.Generation,
}

var objStatus1DiffKind = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     objStatus1.ObjectReference.Group,
		Kind:      "DaemonSet", // Different Kind
		Name:      objStatus1.ObjectReference.Name,
		Namespace: objStatus1.ObjectReference.Namespace,
	},
	Strategy:   objStatus1.Strategy,
	Actuation:  objStatus1.Actuation,
	Reconcile:  objStatus1.Reconcile,
	UID:        objStatus1.UID,
	Generation: objStatus1.Generation,
}

var objStatus1DiffGroup = actuation.ObjectStatus{
	ObjectReference: actuation.ObjectReference{
		Group:     "batch", // Different Group
		Kind:      objStatus1.ObjectReference.Kind,
		Name:      objStatus1.ObjectReference.Name,
		Namespace: objStatus1.ObjectReference.Namespace,
	},
	Strategy:   objStatus1.Strategy,
	Actuation:  objStatus1.Actuation,
	Reconcile:  objStatus1.Reconcile,
	UID:        objStatus1.UID,
	Generation: objStatus1.Generation,
}

func TestObjectStatusSet_Equals(t *testing.T) {
	testCases := map[string]struct {
		setA    ObjectStatusSet
		setB    ObjectStatusSet
		isEqual bool
	}{
		// --- Basic Set Operations ---
		"Empty sets are equal": {
			setA:    ObjectStatusSet{},
			setB:    ObjectStatusSet{},
			isEqual: true,
		},
		"Empty vs non-empty sets are not equal (A empty)": {
			setA:    ObjectStatusSet{},
			setB:    ObjectStatusSet{objStatus1, objStatus3},
			isEqual: false,
		},
		"Empty vs non-empty sets are not equal (B empty)": {
			setA:    ObjectStatusSet{objStatus1, objStatus3},
			setB:    ObjectStatusSet{},
			isEqual: false,
		},
		"Identical non-empty sets are equal": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1, objStatus2},
			isEqual: true,
		},
		"Same elements, different order are equal": {
			setA:    ObjectStatusSet{objStatus2, objStatus1},
			setB:    ObjectStatusSet{objStatus1, objStatus2},
			isEqual: true,
		},
		"Different sizes are not equal": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1},
			isEqual: false,
		},
		"Partial overlap are not equal": {
			setA:    ObjectStatusSet{objStatus2, objStatus1},
			setB:    ObjectStatusSet{objStatus1, objStatus3},
			isEqual: false,
		},
		"Disjoint sets are not equal": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus3, objStatus4},
			isEqual: false,
		},
		"Duplicate elements in one set": {
			setA:    ObjectStatusSet{objStatus1, objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1, objStatus2},
			isEqual: true,
		},
		"Duplicate elements in both sets": {
			setA:    ObjectStatusSet{objStatus1, objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus2, objStatus1, objStatus1},
			isEqual: true,
		},

		// --- Single Field Permutations (Single Element Sets) ---
		"Single item sets, different Strategy": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffStrategy},
			isEqual: false,
		},
		"Single item sets, different Actuation": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffActuation},
			isEqual: false,
		},
		"Single item sets, different Reconcile": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffReconcile},
			isEqual: false,
		},
		"Single item sets, different UID": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffUID},
			isEqual: false,
		},
		"Single item sets, different Generation": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffGeneration},
			isEqual: false,
		},
		"Single item sets, different Name": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffName},
			isEqual: false,
		},
		"Single item sets, different Namespace": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffNamespace},
			isEqual: false,
		},
		"Single item sets, different Kind": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffKind},
			isEqual: false,
		},
		"Single item sets, different Group": {
			setA:    ObjectStatusSet{objStatus1},
			setB:    ObjectStatusSet{objStatus1DiffGroup},
			isEqual: false,
		},

		// --- Single Field Permutations (Multi Element Sets) ---
		"Multi item sets, one item differs by Strategy": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1DiffStrategy, objStatus2},
			isEqual: false,
		},
		"Multi item sets, one item differs by Actuation": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1DiffActuation, objStatus2},
			isEqual: false,
		},
		"Multi item sets, one item differs by Reconcile": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1, objStatus1DiffReconcile},
			isEqual: false,
		},
		"Multi item sets, one item differs by UID": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1, objStatus1DiffUID},
			isEqual: false,
		},
		"Multi item sets, one item differs by Generation": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1DiffGeneration, objStatus2},
			isEqual: false,
		},
		"Multi item sets, one item differs by Name": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1DiffName, objStatus2},
			isEqual: false,
		},
		"Multi item sets, one item differs by Namespace": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1, objStatus1DiffNamespace},
			isEqual: false,
		},
		"Multi item sets, one item differs by Kind": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1DiffKind, objStatus2},
			isEqual: false,
		},
		"Multi item sets, one item differs by Group": {
			setA:    ObjectStatusSet{objStatus1, objStatus2},
			setB:    ObjectStatusSet{objStatus1, objStatus1DiffGroup},
			isEqual: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Test A.Equal(B)
			actualAB := tc.setA.Equal(tc.setB)
			require.Equal(t, tc.isEqual, actualAB, "A.Equal(B) failed")

			// Test B.Equal(A) - ensure commutativity
			actualBA := tc.setB.Equal(tc.setA)
			require.Equal(t, tc.isEqual, actualBA, "B.Equal(A) failed")

			// Test A.Equal(A) and B.Equal(B) - ensure reflexivity
			//nolint:gocritic // argument and receiver deliberately the same
			require.True(t, tc.setA.Equal(tc.setA), "A.Equal(A) failed")
			//nolint:gocritic // argument and receiver deliberately the same
			require.True(t, tc.setB.Equal(tc.setB), "B.Equal(B) failed")
		})
	}
}

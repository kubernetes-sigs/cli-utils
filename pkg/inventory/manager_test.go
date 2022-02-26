// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestObjectStatusGetSet(t *testing.T) {
	manager := NewManager()

	id := object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "group",
			Kind:  "kind",
		},
		Name:      "name",
		Namespace: "namespace",
	}

	// Test get before set
	outStatus, found := manager.ObjectStatus(id)
	require.False(t, found)
	require.Nil(t, outStatus)

	// Test get after set
	inStatus1 := actuation.ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        actuation.ActuationStrategyApply,
		Actuation:       actuation.ActuationPending,
		Reconcile:       actuation.ReconcilePending,
	}
	manager.SetObjectStatus(inStatus1)
	outStatus, found = manager.ObjectStatus(id)
	require.True(t, found)
	require.Equal(t, &inStatus1, outStatus)

	// Test get after re-set
	inStatus2 := actuation.ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        actuation.ActuationStrategyApply,
		Actuation:       actuation.ActuationSucceeded,
		Reconcile:       actuation.ReconcilePending,
	}
	manager.SetObjectStatus(inStatus2)
	outStatus, found = manager.ObjectStatus(id)
	require.True(t, found)
	require.Equal(t, &inStatus2, outStatus)

	// Test get after set via returned pointer
	outStatus.Reconcile = actuation.ReconcileFailed
	outStatus, found = manager.ObjectStatus(id)
	require.True(t, found)
	expStatus := actuation.ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(id),
		Strategy:        actuation.ActuationStrategyApply,
		Actuation:       actuation.ActuationSucceeded,
		Reconcile:       actuation.ReconcileFailed,
	}
	require.Equal(t, &expStatus, outStatus)
}

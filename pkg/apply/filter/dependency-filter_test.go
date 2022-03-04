// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var idInvalid = object.ObjMetadata{
	GroupKind: schema.GroupKind{
		Kind: "", // required
	},
	Name: "invalid", // required
}

var idA = object.ObjMetadata{
	GroupKind: schema.GroupKind{
		Group: "group-a",
		Kind:  "kind-a",
	},
	Name:      "name-a",
	Namespace: "namespace-a",
}

var idB = object.ObjMetadata{
	GroupKind: schema.GroupKind{
		Group: "group-b",
		Kind:  "kind-b",
	},
	Name:      "name-b",
	Namespace: "namespace-b",
}

func TestDependencyFilter(t *testing.T) {
	tests := map[string]struct {
		dryRunStrategy    common.DryRunStrategy
		actuationStrategy actuation.ActuationStrategy
		contextSetup      func(*taskrunner.TaskContext)
		id                object.ObjMetadata
		expectedError     error
	}{
		"apply A (no deps)": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.InventoryManager().AddPendingApply(idA)
			},
			id:            idA,
			expectedError: nil,
		},
		"apply A (A -> B) when B is invalid": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idInvalid)
				taskContext.Graph().AddEdge(idA, idInvalid)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.AddInvalidObject(idInvalid)
			},
			id: idA,
			expectedError: testutil.EqualError(
				NewFatalError(fmt.Errorf("invalid dependency: %s", idInvalid)),
			),
		},
		"apply A (A -> B) before B is applied": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().AddPendingApply(idB)
			},
			id: idA,
			expectedError: testutil.EqualError(
				NewFatalError(fmt.Errorf("premature apply: dependency apply actuation pending: %s", idB)),
			),
		},
		"apply A (A -> B) before B is reconciled": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcilePending,
				})
			},
			id: idA,
			expectedError: testutil.EqualError(
				NewFatalError(fmt.Errorf("premature apply: dependency apply reconcile pending: %s", idB)),
			),
		},
		"apply A (A -> B) after B is reconciled": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileSucceeded,
				})
			},
			id:            idA,
			expectedError: nil,
		},
		"apply A (A -> B) after B apply failed": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationFailed,
					Reconcile:       actuation.ReconcilePending,
				})
			},
			id: idA,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idA,
				Strategy:                actuation.ActuationStrategyApply,
				Relationship:            RelationshipDependency,
				Relation:                idB,
				RelationPhase:           PhaseActuation,
				RelationActuationStatus: actuation.ActuationFailed,
				RelationReconcileStatus: actuation.ReconcilePending,
			},
		},
		"apply A (A -> B) after B apply skipped": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSkipped,
					Reconcile:       actuation.ReconcileSkipped,
				})
			},
			id: idA,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idA,
				Strategy:                actuation.ActuationStrategyApply,
				Relationship:            RelationshipDependency,
				Relation:                idB,
				RelationPhase:           PhaseActuation,
				RelationActuationStatus: actuation.ActuationSkipped,
				RelationReconcileStatus: actuation.ReconcileSkipped,
			},
		},
		"apply A (A -> B) after B reconcile failed": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileFailed,
				})
			},
			id: idA,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idA,
				Strategy:                actuation.ActuationStrategyApply,
				Relationship:            RelationshipDependency,
				Relation:                idB,
				RelationPhase:           PhaseReconcile,
				RelationActuationStatus: actuation.ActuationSucceeded,
				RelationReconcileStatus: actuation.ReconcileFailed,
			},
		},
		"apply A (A -> B) after B reconcile timeout": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileTimeout,
				})
			},
			id: idA,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idA,
				Strategy:                actuation.ActuationStrategyApply,
				Relationship:            RelationshipDependency,
				Relation:                idB,
				RelationPhase:           PhaseReconcile,
				RelationActuationStatus: actuation.ActuationSucceeded,
				RelationReconcileStatus: actuation.ReconcileTimeout,
			},
		},
		// artificial use case: reconcile should only be skipped if apply failed or was skipped
		"apply A (A -> B) after B reconcile skipped": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileSkipped,
				})
			},
			id: idA,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idA,
				Strategy:                actuation.ActuationStrategyApply,
				Relationship:            RelationshipDependency,
				Relation:                idB,
				RelationPhase:           PhaseReconcile,
				RelationActuationStatus: actuation.ActuationSucceeded,
				RelationReconcileStatus: actuation.ReconcileSkipped,
			},
		},
		"apply A (A -> B) when B delete pending": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().AddPendingDelete(idB)
			},
			id: idA,
			expectedError: &DependencyActuationMismatchError{
				Object:           idA,
				Strategy:         actuation.ActuationStrategyApply,
				Relationship:     RelationshipDependency,
				Relation:         idB,
				RelationStrategy: actuation.ActuationStrategyDelete,
			},
		},
		"delete B (no deps)": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
			},
			id:            idB,
			expectedError: nil,
		},
		"delete B (A -> B) when A is invalid": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idInvalid)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idInvalid, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.AddInvalidObject(idInvalid)
			},
			id: idB,
			expectedError: testutil.EqualError(
				NewFatalError(fmt.Errorf("invalid dependent: %s", idInvalid)),
			),
		},
		"delete B (A -> B) before A is deleted": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().AddPendingDelete(idA)
			},
			id: idB,
			expectedError: testutil.EqualError(
				NewFatalError(fmt.Errorf("premature delete: dependent delete actuation pending: %s", idA)),
			),
		},
		"delete B (A -> B) before A is reconciled": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcilePending,
				})
			},
			id: idB,
			expectedError: testutil.EqualError(
				NewFatalError(fmt.Errorf("premature delete: dependent delete reconcile pending: %s", idA)),
			),
		},
		"delete B (A -> B) after A is reconciled": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileSucceeded,
				})
			},
			id:            idB,
			expectedError: nil,
		},
		"delete B (A -> B) after A delete failed": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationFailed,
					Reconcile:       actuation.ReconcilePending,
				})
			},
			id: idB,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idB,
				Strategy:                actuation.ActuationStrategyDelete,
				Relationship:            RelationshipDependent,
				Relation:                idA,
				RelationPhase:           PhaseActuation,
				RelationActuationStatus: actuation.ActuationFailed,
				RelationReconcileStatus: actuation.ReconcilePending,
			},
		},
		"delete B (A -> B) after A delete skipped": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSkipped,
					Reconcile:       actuation.ReconcileSkipped,
				})
			},
			id: idB,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idB,
				Strategy:                actuation.ActuationStrategyDelete,
				Relationship:            RelationshipDependent,
				Relation:                idA,
				RelationPhase:           PhaseActuation,
				RelationActuationStatus: actuation.ActuationSkipped,
				RelationReconcileStatus: actuation.ReconcileSkipped,
			},
		},
		// artificial use case: delete reconcile can't fail, only timeout
		"delete B (A -> B) after A reconcile failed": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileFailed,
				})
			},
			id: idB,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idB,
				Strategy:                actuation.ActuationStrategyDelete,
				Relationship:            RelationshipDependent,
				Relation:                idA,
				RelationPhase:           PhaseReconcile,
				RelationActuationStatus: actuation.ActuationSucceeded,
				RelationReconcileStatus: actuation.ReconcileFailed,
			},
		},
		"delete B (A -> B) after A reconcile timeout": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileTimeout,
				})
			},
			id: idB,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idB,
				Strategy:                actuation.ActuationStrategyDelete,
				Relationship:            RelationshipDependent,
				Relation:                idA,
				RelationPhase:           PhaseReconcile,
				RelationActuationStatus: actuation.ActuationSucceeded,
				RelationReconcileStatus: actuation.ReconcileTimeout,
			},
		},
		// artificial use case: reconcile should only be skipped if delete failed or was skipped
		"delete B (A -> B) after A reconcile skipped": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileSkipped,
				})
			},
			id: idB,
			expectedError: &DependencyPreventedActuationError{
				Object:                  idB,
				Strategy:                actuation.ActuationStrategyDelete,
				Relationship:            RelationshipDependent,
				Relation:                idA,
				RelationPhase:           PhaseReconcile,
				RelationActuationStatus: actuation.ActuationSucceeded,
				RelationReconcileStatus: actuation.ReconcileSkipped,
			},
		},
		"delete B (A -> B) when A apply succeeded": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcileSucceeded,
				})
			},
			id: idB,
			expectedError: &DependencyActuationMismatchError{
				Object:           idB,
				Strategy:         actuation.ActuationStrategyDelete,
				Relationship:     RelationshipDependent,
				Relation:         idA,
				RelationStrategy: actuation.ActuationStrategyApply,
			},
		},
		"DryRun: apply A (A -> B) when B apply reconcile pending": {
			dryRunStrategy:    common.DryRunClient,
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingApply(idA)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idB),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcilePending,
				})
			},
			id:            idA,
			expectedError: nil,
		},
		"DryRun: delete B (A -> B) when A delete reconcile pending": {
			dryRunStrategy:    common.DryRunClient,
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.Graph().AddVertex(idB)
				taskContext.Graph().AddEdge(idA, idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
				taskContext.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idA),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationSucceeded,
					Reconcile:       actuation.ReconcilePending,
				})
			},
			id:            idB,
			expectedError: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			taskContext := taskrunner.NewTaskContext(nil, nil)
			tc.contextSetup(taskContext)

			filter := DependencyFilter{
				TaskContext:       taskContext,
				ActuationStrategy: tc.actuationStrategy,
				DryRunStrategy:    tc.dryRunStrategy,
			}
			obj := defaultObj.DeepCopy()
			obj.SetGroupVersionKind(tc.id.GroupKind.WithVersion("v1"))
			obj.SetName(tc.id.Name)
			obj.SetNamespace(tc.id.Namespace)

			err := filter.Filter(obj)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

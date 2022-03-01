// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
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
		expectedFiltered  bool
		expectedReason    string
		expectedError     error
	}{
		"apply A (no deps)": {
			actuationStrategy: actuation.ActuationStrategyApply,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idA)
				taskContext.InventoryManager().AddPendingApply(idA)
			},
			id:               idA,
			expectedFiltered: false,
			expectedReason:   "",
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependency invalid: %q", idInvalid),
			expectedError:    nil,
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
			id:            idA,
			expectedError: fmt.Errorf("premature apply: dependency apply actuation pending: %q", idB),
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
			id:            idA,
			expectedError: fmt.Errorf("premature apply: dependency apply reconcile pending: %q", idB),
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
			id:               idA,
			expectedFiltered: false,
			expectedReason:   "",
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependency apply actuation failed: %q", idB),
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependency apply actuation skipped: %q", idB),
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependency apply reconcile failed: %q", idB),
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependency apply reconcile timeout: %q", idB),
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependency apply reconcile skipped: %q", idB),
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("apply skipped because dependency is scheduled for delete: %q", idB),
			expectedError:    nil,
		},
		"delete B (no deps)": {
			actuationStrategy: actuation.ActuationStrategyDelete,
			contextSetup: func(taskContext *taskrunner.TaskContext) {
				taskContext.Graph().AddVertex(idB)
				taskContext.InventoryManager().AddPendingDelete(idB)
			},
			id:               idB,
			expectedFiltered: false,
			expectedReason:   "",
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependent invalid: %q", idInvalid),
			expectedError:    nil,
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
			id:            idB,
			expectedError: fmt.Errorf("premature delete: dependent delete actuation pending: %q", idA),
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
			id:            idB,
			expectedError: fmt.Errorf("premature delete: dependent delete reconcile pending: %q", idA),
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
			id:               idB,
			expectedFiltered: false,
			expectedReason:   "",
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependent delete actuation failed: %q", idA),
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependent delete actuation skipped: %q", idA),
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependent delete reconcile failed: %q", idA),
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependent delete reconcile timeout: %q", idA),
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("dependent delete reconcile skipped: %q", idA),
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: true,
			expectedReason:   fmt.Sprintf("delete skipped because dependent is scheduled for apply: %q", idA),
			expectedError:    nil,
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
			id:               idA,
			expectedFiltered: false,
			expectedReason:   "",
			expectedError:    nil,
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
			id:               idB,
			expectedFiltered: false,
			expectedReason:   "",
			expectedError:    nil,
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

			filtered, reason, err := filter.Filter(obj)
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectedFiltered, filtered)
			assert.Equal(t, tc.expectedReason, reason)
		})
	}
}

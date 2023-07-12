// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestDeleteInvTask(t *testing.T) {
	id1 := object.UnstructuredToObjMetadata(obj1)
	id2 := object.UnstructuredToObjMetadata(obj2)
	id3 := object.UnstructuredToObjMetadata(obj3)
	testCases := map[string]struct {
		prevInventory     object.ObjMetadataSet
		failedDeletes     object.ObjMetadataSet
		failedReconciles  object.ObjMetadataSet
		timeoutReconciles object.ObjMetadataSet
		err               error
		isError           bool
		expectedObjs      object.ObjMetadataSet
	}{
		"no error case": {
			prevInventory: object.ObjMetadataSet{id1, id2, id3},
			err:           nil,
			isError:       false,
		},
		"error is returned in result": {
			err:     apierrors.NewResourceExpired("unused message"),
			isError: true,
		},
		"inventory not found is not error and not returned": {
			err: apierrors.NewNotFound(schema.GroupResource{Resource: "simples"},
				"unused-resource-name"),
			isError: false,
		},
		"inventory is updated instead of deleted in case of pruning failure": {
			prevInventory: object.ObjMetadataSet{id1, id2, id3},
			failedDeletes: object.ObjMetadataSet{id1},
			err:           nil,
			isError:       false,
			expectedObjs:  object.ObjMetadataSet{id1},
		},
		"inventory is updated instead of deleted in case of reconcile failure": {
			prevInventory:    object.ObjMetadataSet{id1, id2, id3},
			failedReconciles: object.ObjMetadataSet{id1},
			err:              nil,
			isError:          false,
			expectedObjs:     object.ObjMetadataSet{id1},
		},
		"inventory is updated instead of deleted in case of reconcile timeout": {
			prevInventory:     object.ObjMetadataSet{id1, id2, id3},
			timeoutReconciles: object.ObjMetadataSet{id1},
			err:               nil,
			isError:           false,
			expectedObjs:      object.ObjMetadataSet{id1},
		},
		"inventory is updated instead of deleted in case of pruning/reconcile failure": {
			prevInventory:    object.ObjMetadataSet{id1, id2, id3},
			failedReconciles: object.ObjMetadataSet{id1},
			failedDeletes:    object.ObjMetadataSet{id2},
			err:              nil,
			isError:          false,
			expectedObjs:     object.ObjMetadataSet{id1, id2},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			client := inventory.NewFakeClient(object.ObjMetadataSet{})
			client.Err = tc.err
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			context := taskrunner.NewTaskContext(eventChannel, resourceCache)
			for _, failedDelete := range tc.failedDeletes {
				context.InventoryManager().AddFailedDelete(failedDelete)
			}
			for _, failedReconcile := range tc.failedReconciles {
				context.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: actuation.ObjectReference{
						Group:     failedReconcile.GroupKind.Group,
						Kind:      failedReconcile.GroupKind.Kind,
						Name:      failedReconcile.Name,
						Namespace: failedReconcile.Namespace,
					},
				})
				context.AddInvalidObject(failedReconcile)
				if err := context.InventoryManager().SetFailedReconcile(failedReconcile); err != nil {
					t.Fatal(err)
				}
			}
			for _, timeoutReconcile := range tc.timeoutReconciles {
				context.InventoryManager().SetObjectStatus(actuation.ObjectStatus{
					ObjectReference: actuation.ObjectReference{
						Group:     timeoutReconcile.GroupKind.Group,
						Kind:      timeoutReconcile.GroupKind.Kind,
						Name:      timeoutReconcile.Name,
						Namespace: timeoutReconcile.Namespace,
					},
				})
				context.AddInvalidObject(timeoutReconcile)
				if err := context.InventoryManager().SetTimeoutReconcile(timeoutReconcile); err != nil {
					t.Fatal(err)
				}
			}

			task := DeleteOrUpdateInvTask{
				TaskName:      taskName,
				InvClient:     client,
				InvInfo:       nil,
				DryRun:        common.DryRunNone,
				PrevInventory: tc.prevInventory,
				Destroy:       true,
			}
			if taskName != task.Name() {
				t.Errorf("expected task name (%s), got (%s)", taskName, task.Name())
			}
			task.Start(context)
			result := <-context.TaskChannel()
			if tc.isError {
				if tc.err != result.Err {
					t.Errorf("running DeleteOrUpdateInvTask expected error (%s), got (%s)", tc.err, result.Err)
				}
			} else {
				if result.Err != nil {
					t.Errorf("unexpected error running DeleteOrUpdateInvTask: %s", result.Err)
				}
			}
			actual, _ := client.GetClusterObjs(nil)
			testutil.AssertEqual(t, tc.expectedObjs, actual,
				"Actual cluster objects (%d) do not match expected cluster objects (%d)",
				len(actual), len(tc.expectedObjs))
		})
	}
}

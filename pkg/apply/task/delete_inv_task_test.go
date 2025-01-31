// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
		deletedObjs       object.ObjMetadataSet
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
			deletedObjs:      object.ObjMetadataSet{id1, id2, id3},
			failedReconciles: object.ObjMetadataSet{id1},
			err:              nil,
			isError:          false,
			expectedObjs:     object.ObjMetadataSet{id1},
		},
		"inventory is updated instead of deleted in case of reconcile timeout": {
			prevInventory:     object.ObjMetadataSet{id1, id2, id3},
			deletedObjs:       object.ObjMetadataSet{id1, id2, id3},
			timeoutReconciles: object.ObjMetadataSet{id1},
			err:               nil,
			isError:           false,
			expectedObjs:      object.ObjMetadataSet{id1},
		},
		"inventory is updated instead of deleted in case of pruning/reconcile failure": {
			prevInventory:    object.ObjMetadataSet{id1, id2, id3},
			deletedObjs:      object.ObjMetadataSet{id1, id2, id3},
			failedReconciles: object.ObjMetadataSet{id1},
			failedDeletes:    object.ObjMetadataSet{id2},
			err:              nil,
			isError:          false,
			expectedObjs:     object.ObjMetadataSet{id1, id2},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			client := inventory.NewFakeClient(tc.prevInventory)
			client.Err = tc.err
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := taskrunner.NewTaskContext(t.Context(), eventChannel, resourceCache)
			im := taskContext.InventoryManager()
			for _, deleteObj := range tc.deletedObjs {
				im.AddSuccessfulDelete(deleteObj, "unused-uid")
			}
			for _, failedDelete := range tc.failedDeletes {
				im.AddFailedDelete(failedDelete)
			}
			for _, failedReconcile := range tc.failedReconciles {
				if err := im.SetFailedReconcile(failedReconcile); err != nil {
					t.Fatal(err)
				}
			}
			for _, timeoutReconcile := range tc.timeoutReconciles {
				if err := im.SetTimeoutReconcile(timeoutReconcile); err != nil {
					t.Fatal(err)
				}
			}

			task := DeleteOrUpdateInvTask{
				TaskName:         taskName,
				InvClient:        client,
				InvInfo:          nil,
				DryRun:           common.DryRunNone,
				ClusterInventory: client.Inv,
				Destroy:          true,
			}
			if taskName != task.Name() {
				t.Errorf("expected task name (%s), got (%s)", taskName, task.Name())
			}
			task.Start(taskContext)
			result := <-taskContext.TaskChannel()
			if tc.isError {
				if tc.err != result.Err {
					t.Errorf("running DeleteOrUpdateInvTask expected error (%s), got (%s)", tc.err, result.Err)
				}
				return
			}
			if result.Err != nil {
				t.Errorf("unexpected error running DeleteOrUpdateInvTask: %s", result.Err)
			}
			actual, _ := client.Get(context.TODO(), nil, inventory.GetOptions{})
			if len(tc.expectedObjs) > 0 {
				testutil.AssertEqual(t, tc.expectedObjs, actual.Objects(),
					"Actual cluster objects (%d) do not match expected cluster objects (%d)",
					len(actual.Objects()), len(tc.expectedObjs))
			}
		})
	}
}

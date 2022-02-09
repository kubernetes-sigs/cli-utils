// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestDeleteInvTask(t *testing.T) {
	ref1 := inventory.ObjectReferenceFromObject(obj1)
	// ref2 := inventory.ObjectReferenceFromObject(obj2)
	// ref3 := inventory.ObjectReferenceFromObject(obj3)

	invTypeMeta := v1.TypeMeta{
		APIVersion: inventoryObj.GetAPIVersion(),
		Kind:       inventoryObj.GetKind(),
	}
	invObjMeta := v1.ObjectMeta{
		Name:      inventoryObj.GetName(),
		Namespace: inventoryObj.GetNamespace(),
		Labels:    inventoryObj.GetLabels(),
	}

	testCases := map[string]struct {
		err               error
		contextInventory  *actuation.Inventory
		expectedError     error
		expectedInventory *actuation.Inventory
	}{
		"no error case": {
			err: nil,
			contextInventory: &actuation.Inventory{
				TypeMeta:   invTypeMeta,
				ObjectMeta: invObjMeta,
			},
			expectedError:     nil,
			expectedInventory: nil,
		},
		"one succeeded delete; no inventory object": {
			contextInventory: &actuation.Inventory{
				TypeMeta:   invTypeMeta,
				ObjectMeta: invObjMeta,
				Spec:       actuation.InventorySpec{},
				Status: actuation.InventoryStatus{
					Objects: []actuation.ObjectStatus{
						{
							ObjectReference: ref1,
							Strategy:        actuation.ActuationStrategyDelete,
							Actuation:       actuation.ActuationSucceeded,
							Reconcile:       actuation.ReconcileSucceeded,
						},
					},
				},
			},
			expectedInventory: nil,
		},
		"one failed delete; one inventory object": {
			contextInventory: &actuation.Inventory{
				TypeMeta:   invTypeMeta,
				ObjectMeta: invObjMeta,
				Spec:       actuation.InventorySpec{},
				Status: actuation.InventoryStatus{
					Objects: []actuation.ObjectStatus{
						{
							ObjectReference: ref1,
							Strategy:        actuation.ActuationStrategyDelete,
							Actuation:       actuation.ActuationFailed,
							Reconcile:       actuation.ReconcileSkipped,
						},
					},
				},
			},
			expectedInventory: &actuation.Inventory{
				TypeMeta:   invTypeMeta,
				ObjectMeta: invObjMeta,
				Spec: actuation.InventorySpec{
					Objects: []actuation.ObjectReference{ref1},
				},
				Status: actuation.InventoryStatus{
					Objects: []actuation.ObjectStatus{
						{
							ObjectReference: ref1,
							Strategy:        actuation.ActuationStrategyDelete,
							Actuation:       actuation.ActuationFailed,
							Reconcile:       actuation.ReconcileSkipped,
						},
					},
				},
			},
		},
		// TODO: make a FakeClient to test error handling
		// "error is returned in result": {
		// 	err:     apierrors.NewResourceExpired("unused message"),
		// 	isError: true,
		// },
		// "inventory not found is not error and not returned": {
		// 	err: apierrors.NewNotFound(schema.GroupResource{Resource: "simples"},
		// 		"unused-resource-name"),
		// 	isError: false,
		// },
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			client := &inventory.InMemoryClient{}
			//client.Err = tc.err
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := taskrunner.NewTaskContext(eventChannel, resourceCache)

			// initialize inventory in context
			inv := taskContext.InventoryManager().Inventory()
			tc.contextInventory.DeepCopyInto(inv)

			task := DeleteInvTask{
				TaskName:  taskName,
				InvClient: client,
				DryRun:    common.DryRunNone,
			}
			if taskName != task.Name() {
				t.Errorf("expected task name (%s), got (%s)", taskName, task.Name())
			}
			task.Start(taskContext)
			result := <-taskContext.TaskChannel()
			if tc.expectedError != nil {
				assert.EqualError(t, result.Err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, result.Err)

			inv, err := client.Load(context.TODO(), inventoryInfo)
			assert.NoError(t, err)
			testutil.AssertEqual(t, tc.expectedInventory, inv)
		})
	}
}

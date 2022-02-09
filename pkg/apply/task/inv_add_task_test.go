// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var namespace = "test-namespace"

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "test-inventory-obj",
			"namespace": namespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: "test-app-label",
			},
		},
	},
}

var inventoryInfo = inventory.InventoryInfoFromObject(inventoryObj)

var obj1 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "obj1",
			"namespace": namespace,
		},
	},
}

var obj2 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      "obj2",
			"namespace": namespace,
		},
	},
}

var obj3 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "obj3",
			"namespace": "different-namespace",
		},
	},
}

const taskName = "test-inventory-task"

func TestInvAddTask(t *testing.T) {
	id1 := object.UnstructuredToObjMetadata(obj1)
	id2 := object.UnstructuredToObjMetadata(obj2)
	id3 := object.UnstructuredToObjMetadata(obj3)
	ref1 := inventory.ObjectReferenceFromObject(obj1)
	ref2 := inventory.ObjectReferenceFromObject(obj2)
	ref3 := inventory.ObjectReferenceFromObject(obj3)

	tests := map[string]struct {
		applyObjs          []*unstructured.Unstructured
		pruneObjs          []*unstructured.Unstructured
		expectedIds        object.ObjMetadataSet
		expectedSpecObjs   []actuation.ObjectReference
		expectedStatusObjs []actuation.ObjectStatus
		expectedError      error
	}{
		"no prune objects, no apply objects; no  inventory": {
			applyObjs:          []*unstructured.Unstructured{},
			pruneObjs:          []*unstructured.Unstructured{},
			expectedIds:        object.ObjMetadataSet{},
			expectedSpecObjs:   []actuation.ObjectReference{},
			expectedStatusObjs: []actuation.ObjectStatus{},
		},
		"no prune objects, one apply object; one merged inventory": {
			applyObjs:        []*unstructured.Unstructured{obj1},
			pruneObjs:        []*unstructured.Unstructured{},
			expectedIds:      object.ObjMetadataSet{id1},
			expectedSpecObjs: []actuation.ObjectReference{ref1},
			expectedStatusObjs: []actuation.ObjectStatus{
				{
					ObjectReference: ref1,
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
		},
		"one prune object, no apply object; one merged inventory": {
			applyObjs:        []*unstructured.Unstructured{},
			pruneObjs:        []*unstructured.Unstructured{obj2},
			expectedIds:      object.ObjMetadataSet{id2},
			expectedSpecObjs: []actuation.ObjectReference{ref2},
			expectedStatusObjs: []actuation.ObjectStatus{
				{
					ObjectReference: ref2,
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
		},
		"one prune object, one apply object; one merged inventory": {
			applyObjs:     []*unstructured.Unstructured{obj3},
			pruneObjs:     []*unstructured.Unstructured{obj3},
			expectedIds:   object.ObjMetadataSet{id3},
			expectedError: fmt.Errorf("apply set and delete set share objects: %v", object.ObjMetadataSet{id3}),
		},
		"three prune objects, two same objects; three merged inventory": {
			applyObjs:        []*unstructured.Unstructured{obj2, obj3},
			pruneObjs:        []*unstructured.Unstructured{obj1},
			expectedIds:      object.ObjMetadataSet{id2, id3, id1},
			expectedSpecObjs: []actuation.ObjectReference{ref2, ref3, ref1},
			expectedStatusObjs: []actuation.ObjectStatus{
				{
					ObjectReference: ref2,
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
				{
					ObjectReference: ref3,
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
				{
					ObjectReference: ref1,
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := &inventory.InMemoryClient{}
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := taskrunner.NewTaskContext(eventChannel, resourceCache)

			// initialize inventory reference in context
			inv := taskContext.InventoryManager().Inventory()
			inv.SetGroupVersionKind(inventoryObj.GroupVersionKind())
			inv.SetName(inventoryObj.GetName())
			inv.SetNamespace(inventoryObj.GetNamespace())
			inv.SetLabels(inventoryObj.GetLabels())

			task := InvAddTask{
				TaskName:     taskName,
				InvClient:    client,
				ApplyObjects: tc.applyObjs,
				PruneObjects: tc.pruneObjs,
			}
			assert.Equal(t, taskName, task.Name())
			testutil.AssertEqual(t, tc.expectedIds, task.Identifiers())

			task.Start(taskContext)
			result := <-taskContext.TaskChannel()
			if tc.expectedError != nil {
				assert.EqualError(t, result.Err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, result.Err)

			inv, err := client.Load(context.TODO(), inventoryInfo)
			assert.NoError(t, err)
			testutil.AssertEqual(t, tc.expectedSpecObjs, inv.Spec.Objects)
			testutil.AssertEqual(t, tc.expectedStatusObjs, inv.Status.Objects)
		})
	}
}

func TestInventoryNamespaceInSet(t *testing.T) {
	inventoryNamespace := createNamespace(namespace)

	tests := map[string]struct {
		inv       client.Object
		objects   []*unstructured.Unstructured
		namespace *unstructured.Unstructured
	}{
		"empty inventory reference, no resources returns nil namespace": {
			inv:       &unstructured.Unstructured{},
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, but no resources returns nil namespace": {
			inv:       inventoryObj,
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, resources with no namespace returns nil namespace": {
			inv:       inventoryObj,
			objects:   []*unstructured.Unstructured{obj1, obj2},
			namespace: nil,
		},
		"Inventory object, different namespace returns nil namespace": {
			inv:       inventoryObj,
			objects:   []*unstructured.Unstructured{createNamespace("foo")},
			namespace: nil,
		},
		"Inventory object, inventory namespace returns inventory namespace": {
			inv:       inventoryObj,
			objects:   []*unstructured.Unstructured{obj1, inventoryNamespace, obj3},
			namespace: inventoryNamespace,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualNamespace := inventoryNamespaceInSet(tc.inv, tc.objects)
			if tc.namespace != actualNamespace {
				t.Fatalf("expected namespace (%v), got (%v)", tc.namespace, actualNamespace)
			}
		})
	}
}

func createNamespace(ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": ns,
			},
		},
	}
}

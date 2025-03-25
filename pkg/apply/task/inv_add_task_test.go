// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      inventory.TestInventoryName,
			"namespace": inventory.TestInventoryNamespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: "test-app-label",
			},
		},
	},
}

var obj1 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "obj1",
			"namespace": inventory.TestInventoryNamespace,
		},
	},
}

var obj2 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      "obj2",
			"namespace": inventory.TestInventoryNamespace,
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

var nsObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": inventory.TestInventoryNamespace,
		},
	},
}

const taskName = "test-inventory-task"

func TestInvAddTask(t *testing.T) {
	id1 := object.UnstructuredToObjMetadata(obj1)
	id2 := object.UnstructuredToObjMetadata(obj2)
	id3 := object.UnstructuredToObjMetadata(obj3)
	idNs := object.UnstructuredToObjMetadata(nsObj)

	tests := map[string]struct {
		initialObjs           object.ObjMetadataSet
		applyObjs             []*unstructured.Unstructured
		expectedObjs          object.ObjMetadataSet
		expectedObjStatuses   []actuation.ObjectStatus
		reactorError          error
		expectCreateNamespace bool
	}{
		"no initial inventory and no apply objects; no merged inventory": {
			initialObjs:  object.ObjMetadataSet{},
			applyObjs:    []*unstructured.Unstructured{},
			expectedObjs: object.ObjMetadataSet{},
		},
		"no initial inventory, one apply object; one merged inventory": {
			initialObjs:  object.ObjMetadataSet{},
			applyObjs:    []*unstructured.Unstructured{obj1},
			expectedObjs: object.ObjMetadataSet{id1},
			expectedObjStatuses: []actuation.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(id1),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
		},
		"one initial inventory, no apply object; one merged inventory": {
			initialObjs:  object.ObjMetadataSet{id2},
			applyObjs:    []*unstructured.Unstructured{},
			expectedObjs: object.ObjMetadataSet{id2},
			expectedObjStatuses: []actuation.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(id2),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
		},
		"one initial inventory, one apply object; one merged inventory": {
			initialObjs:  object.ObjMetadataSet{id3},
			applyObjs:    []*unstructured.Unstructured{obj3},
			expectedObjs: object.ObjMetadataSet{id3},
			expectedObjStatuses: []actuation.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(id3),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
		},
		"three initial inventory, two same objects; three merged inventory": {
			initialObjs:  object.ObjMetadataSet{id1, id2, id3},
			applyObjs:    []*unstructured.Unstructured{obj2, obj3},
			expectedObjs: object.ObjMetadataSet{id1, id2, id3},
			expectedObjStatuses: []actuation.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(id1),
					Strategy:        actuation.ActuationStrategyDelete,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(id2),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(id3),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
		},
		"namespace of inventory inside inventory": {
			initialObjs:  object.ObjMetadataSet{},
			applyObjs:    []*unstructured.Unstructured{nsObj},
			expectedObjs: object.ObjMetadataSet{idNs},
			expectedObjStatuses: []actuation.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(idNs),
					Strategy:        actuation.ActuationStrategyApply,
					Actuation:       actuation.ActuationPending,
					Reconcile:       actuation.ReconcilePending,
				},
			},
			expectCreateNamespace: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := inventory.NewFakeClient(tc.initialObjs)
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := taskrunner.NewTaskContext(t.Context(), eventChannel, resourceCache)
			tf := cmdtesting.NewTestFactory().WithNamespace(inventory.TestInventoryNamespace)
			defer tf.Cleanup()

			createdNamespace := false
			tf.FakeDynamicClient.PrependReactor("create", "namespaces", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				createdNamespace = true
				return true, nil, tc.reactorError
			})

			mapper, err := tf.ToRESTMapper()
			if err != nil {
				t.Fatal(err)
			}

			task := InvAddTask{
				TaskName:      taskName,
				InvClient:     client,
				Inventory:     client.Inv,
				Objects:       tc.applyObjs,
				DynamicClient: tf.FakeDynamicClient,
				Mapper:        mapper,
			}
			if taskName != task.Name() {
				t.Errorf("expected task name (%s), got (%s)", taskName, task.Name())
			}
			applyIDs := object.UnstructuredSetToObjMetadataSet(tc.applyObjs)
			if !task.Identifiers().Equal(applyIDs) {
				t.Errorf("expected task ids (%s), got (%s)", applyIDs, task.Identifiers())
			}
			task.Start(taskContext)
			result := <-taskContext.TaskChannel()
			if result.Err != nil {
				t.Errorf("unexpected error running InvAddTask: %s", result.Err)
			}
			// argument doesn't matter for fake client, it always returns cached obj
			actual, _ := client.Get(t.Context(), nil, inventory.GetOptions{})
			if !tc.expectedObjs.Equal(actual.GetObjectRefs()) {
				t.Errorf("expected merged inventory (%s), got (%s)", tc.expectedObjs, actual)
			}
			if !equality.Semantic.DeepEqual(tc.expectedObjStatuses, actual.GetObjectStatuses()) {
				t.Errorf("expected object statuses (%v), got (%v)", tc.expectedObjStatuses, actual.GetObjectStatuses())
			}
			if createdNamespace != tc.expectCreateNamespace {
				t.Errorf("expected create namespace %v, got %v", tc.expectCreateNamespace, createdNamespace)
			}
		})
	}
}

func TestInventoryNamespaceInSet(t *testing.T) {
	localInv, err := inventory.ConfigMapToInventoryInfo(inventoryObj)
	require.NoError(t, err)
	inventoryNamespace := createNamespace(inventory.TestInventoryNamespace)

	tests := map[string]struct {
		inv       inventory.Info
		objects   []*unstructured.Unstructured
		namespace *unstructured.Unstructured
	}{
		"Nil inventory object, no resources returns nil namespace": {
			inv:       nil,
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, but no resources returns nil namespace": {
			inv:       localInv,
			objects:   []*unstructured.Unstructured{},
			namespace: nil,
		},
		"Inventory object, resources with no namespace returns nil namespace": {
			inv:       localInv,
			objects:   []*unstructured.Unstructured{obj1, obj2},
			namespace: nil,
		},
		"Inventory object, different namespace returns nil namespace": {
			inv:       localInv,
			objects:   []*unstructured.Unstructured{createNamespace("foo")},
			namespace: nil,
		},
		"Inventory object, inventory namespace returns inventory namespace": {
			inv:       localInv,
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

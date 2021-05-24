// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
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

var localInv = inventory.WrapInventoryInfoObj(inventoryObj)

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

func TestInvAddTask(t *testing.T) {
	id1 := object.UnstructuredToObjMeta(obj1)
	id2 := object.UnstructuredToObjMeta(obj2)
	id3 := object.UnstructuredToObjMeta(obj3)

	tests := map[string]struct {
		initialObjs  []object.ObjMetadata
		applyObjs    []*unstructured.Unstructured
		expectedObjs []object.ObjMetadata
	}{
		"no initial inventory and no apply objects; no merged inventory": {
			initialObjs:  []object.ObjMetadata{},
			applyObjs:    []*unstructured.Unstructured{},
			expectedObjs: []object.ObjMetadata{},
		},
		"no initial inventory, one apply object; one merged inventory": {
			initialObjs:  []object.ObjMetadata{},
			applyObjs:    []*unstructured.Unstructured{obj1},
			expectedObjs: []object.ObjMetadata{id1},
		},
		"one initial inventory, no apply object; one merged inventory": {
			initialObjs:  []object.ObjMetadata{id2},
			applyObjs:    []*unstructured.Unstructured{},
			expectedObjs: []object.ObjMetadata{id2},
		},
		"one initial inventory, one apply object; one merged inventory": {
			initialObjs:  []object.ObjMetadata{id3},
			applyObjs:    []*unstructured.Unstructured{obj3},
			expectedObjs: []object.ObjMetadata{id3},
		},
		"three initial inventory, two same objects; three merged inventory": {
			initialObjs:  []object.ObjMetadata{id1, id2, id3},
			applyObjs:    []*unstructured.Unstructured{obj2, obj3},
			expectedObjs: []object.ObjMetadata{id1, id2, id3},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := inventory.NewFakeInventoryClient(tc.initialObjs)
			eventChannel := make(chan event.Event)
			context := taskrunner.NewTaskContext(eventChannel)
			task := InvAddTask{
				InvClient: client,
				InvInfo:   nil,
				Objects:   tc.applyObjs,
			}
			if task.Name() != "inventory-add" {
				t.Errorf("expected task name (inventory-add), got (%s)", task.Name())
			}
			applyIds := object.UnstructuredsToObjMetas(tc.applyObjs)
			if !object.SetEquals(applyIds, task.Identifiers()) {
				t.Errorf("exptected task ids (%s), got (%s)", applyIds, task.Identifiers())
			}
			task.Start(context)
			result := <-context.TaskChannel()
			if result.Err != nil {
				t.Errorf("unexpected error running InvAddTask: %s", result.Err)
			}
			actual, _ := client.GetClusterObjs(nil)
			if !object.SetEquals(tc.expectedObjs, actual) {
				t.Errorf("expected merged inventory (%s), got (%s)", tc.expectedObjs, actual)
			}
		})
	}
}

func TestInventoryNamespaceInSet(t *testing.T) {
	inventoryNamespace := createNamespace(namespace)

	tests := map[string]struct {
		inv       inventory.InventoryInfo
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

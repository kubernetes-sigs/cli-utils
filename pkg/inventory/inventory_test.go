// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var testNamespace = "test-inventory-namespace"
var inventoryObjName = "test-inventory-obj"
var pod1Name = "pod-1"
var pod2Name = "pod-2"
var pod3Name = "pod-3"

var testInventoryLabel = "test-app-label"

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      inventoryObjName,
			"namespace": testNamespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: testInventoryLabel,
			},
		},
	},
}

var legacyInvObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      legacyInvName,
			"namespace": testNamespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: testInventoryLabel,
			},
		},
	},
}

var invInfo = &resource.Info{
	Namespace: testNamespace,
	Name:      inventoryObjName,
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: inventoryObj,
}

var pod1 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      pod1Name,
			"namespace": testNamespace,
			"uid":       "uid1",
		},
	},
}

var pod2 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      pod2Name,
			"namespace": testNamespace,
			"uid":       "uid2",
		},
	},
}

var pod2Info = &resource.Info{
	Namespace: testNamespace,
	Name:      pod2Name,
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: pod2,
}

var pod3 = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      pod3Name,
			"namespace": testNamespace,
			"uid":       "uid3",
		},
	},
}

func TestFindInventoryObj(t *testing.T) {
	tests := map[string]struct {
		infos  []*unstructured.Unstructured
		exists bool
		name   string
	}{
		"No inventory object is false": {
			infos:  []*unstructured.Unstructured{},
			exists: false,
			name:   "",
		},
		"Nil inventory object is false": {
			infos:  []*unstructured.Unstructured{nil},
			exists: false,
			name:   "",
		},
		"Only inventory object is true": {
			infos:  []*unstructured.Unstructured{copyInventoryInfo()},
			exists: true,
			name:   inventoryObjName,
		},
		"Missing inventory object is false": {
			infos:  []*unstructured.Unstructured{pod1},
			exists: false,
			name:   "",
		},
		"Multiple non-inventory objects is false": {
			infos:  []*unstructured.Unstructured{pod1, pod2, pod3},
			exists: false,
			name:   "",
		},
		"Inventory object with multiple others is true": {
			infos:  []*unstructured.Unstructured{pod1, pod2, copyInventoryInfo(), pod3},
			exists: true,
			name:   inventoryObjName,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			inventoryObj := FindInventoryObj(tc.infos)
			if tc.exists && inventoryObj == nil {
				t.Errorf("Should have found inventory object")
			}
			if !tc.exists && inventoryObj != nil {
				t.Errorf("Inventory object found, but it does not exist: %#v", inventoryObj)
			}
			if tc.exists && inventoryObj != nil && tc.name != inventoryObj.GetName() {
				t.Errorf("Inventory object name does not match: %s/%s", tc.name, inventoryObj.GetName())
			}
		})
	}
}

func TestIsInventoryObject(t *testing.T) {
	tests := []struct {
		invInfo     *resource.Info
		isInventory bool
	}{
		{
			invInfo:     invInfo,
			isInventory: true,
		},
		{
			invInfo:     pod2Info,
			isInventory: false,
		},
	}

	for _, test := range tests {
		inventory := IsInventoryObject(test.invInfo.Object.(*unstructured.Unstructured))
		if test.isInventory && !inventory {
			t.Errorf("Inventory object not identified: %#v", test.invInfo)
		}
		if !test.isInventory && inventory {
			t.Errorf("Non-inventory object identifed as inventory obj: %#v", test.invInfo)
		}
	}
}

func TestRetrieveInventoryLabel(t *testing.T) {
	tests := []struct {
		inventoryInfo  *resource.Info
		inventoryLabel string
		isError        bool
	}{
		// Pod is not a inventory object.
		{
			inventoryInfo:  pod2Info,
			inventoryLabel: "",
			isError:        true,
		},
		{
			inventoryInfo:  invInfo,
			inventoryLabel: testInventoryLabel,
			isError:        false,
		},
	}

	for _, test := range tests {
		actual := InventoryLabel(object.InfoToUnstructured(test.inventoryInfo))
		assert.Equal(t, test.inventoryLabel, actual)
	}
}

func TestSplitUnstructureds(t *testing.T) {
	tests := map[string]struct {
		allObjs      []*unstructured.Unstructured
		expectedInv  *unstructured.Unstructured
		expectedObjs []*unstructured.Unstructured
		isError      bool
	}{
		"No objects is returns nil and no objects": {
			allObjs:      []*unstructured.Unstructured{},
			expectedInv:  nil,
			expectedObjs: []*unstructured.Unstructured{},
			isError:      false,
		},
		"Only inventory object returns inv and no objects": {
			allObjs:      []*unstructured.Unstructured{inventoryObj},
			expectedInv:  inventoryObj,
			expectedObjs: []*unstructured.Unstructured{},
			isError:      false,
		},
		"Single object returns nil inventory and object": {
			allObjs:      []*unstructured.Unstructured{pod1},
			expectedInv:  nil,
			expectedObjs: []*unstructured.Unstructured{pod1},
			isError:      false,
		},
		"Multiple non-inventory objects returns nil inventory and objs": {
			allObjs:      []*unstructured.Unstructured{pod1, pod2, pod3},
			expectedInv:  nil,
			expectedObjs: []*unstructured.Unstructured{pod1, pod2, pod3},
			isError:      false,
		},
		"Inventory object with multiple others splits correctly": {
			allObjs:      []*unstructured.Unstructured{pod1, pod2, inventoryObj, pod3},
			expectedInv:  inventoryObj,
			expectedObjs: []*unstructured.Unstructured{pod1, pod2, pod3},
			isError:      false,
		},
		"Multiple inventory objects returns error": {
			allObjs:      []*unstructured.Unstructured{pod1, legacyInvObj, inventoryObj, pod3},
			expectedInv:  nil,
			expectedObjs: []*unstructured.Unstructured{},
			isError:      true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actualInv, actualObjs, err := SplitUnstructureds(tc.allObjs)
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error not received")
				}
				return
			}
			if tc.expectedInv != actualInv {
				t.Errorf("expected inventory object (%v), got (%v)", tc.expectedInv, actualInv)
			}
			if len(tc.expectedObjs) != len(actualObjs) {
				t.Errorf("expected %d objects; got %d", len(tc.expectedObjs), len(actualObjs))
			}
		})
	}
}

func copyInventoryInfo() *unstructured.Unstructured {
	return inventoryObj.DeepCopy()
}

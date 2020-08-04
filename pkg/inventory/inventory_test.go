// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"
	"regexp"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

var inventoryObj = unstructured.Unstructured{
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

var legacyInvObj = unstructured.Unstructured{
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
	Object: &inventoryObj,
}

var pod1 = unstructured.Unstructured{
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

var pod1Info = &resource.Info{
	Namespace: testNamespace,
	Name:      pod1Name,
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: &pod1,
}

var pod2 = unstructured.Unstructured{
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
	Object: &pod2,
}

var pod3 = unstructured.Unstructured{
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

var pod3Info = &resource.Info{
	Namespace: testNamespace,
	Name:      pod3Name,
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: &pod3,
}

var nilInfo = &resource.Info{
	Namespace: testNamespace,
	Name:      inventoryObjName,
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: nil,
}

var inventoryObjLabelWithSpace = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      inventoryObjName,
			"namespace": testNamespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: "\tinventory-label ",
			},
		},
	},
}

var invInfoLabelWithSpace = &resource.Info{
	Namespace: testNamespace,
	Name:      inventoryObjName,
	Mapping: &meta.RESTMapping{
		Scope: meta.RESTScopeNamespace,
	},
	Object: &inventoryObjLabelWithSpace,
}

func TestFindInventoryObj(t *testing.T) {
	tests := map[string]struct {
		infos  []*resource.Info
		exists bool
		name   string
	}{
		"No inventory object is false": {
			infos:  []*resource.Info{},
			exists: false,
			name:   "",
		},
		"Nil inventory object is false": {
			infos:  []*resource.Info{nil},
			exists: false,
			name:   "",
		},
		"Only inventory object is true": {
			infos:  []*resource.Info{copyInventoryInfo()},
			exists: true,
			name:   inventoryObjName,
		},
		"Missing inventory object is false": {
			infos:  []*resource.Info{pod1Info},
			exists: false,
			name:   "",
		},
		"Multiple non-inventory objects is false": {
			infos:  []*resource.Info{pod1Info, pod2Info, pod3Info},
			exists: false,
			name:   "",
		},
		"Inventory object with multiple others is true": {
			infos:  []*resource.Info{pod1Info, pod2Info, copyInventoryInfo(), pod3Info},
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
			if tc.exists && inventoryObj != nil && tc.name != inventoryObj.Name {
				t.Errorf("Inventory object name does not match: %s/%s", tc.name, inventoryObj.Name)
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
			invInfo:     nil,
			isInventory: false,
		},
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
		inventory := IsInventoryObject(test.invInfo)
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
		// Nil inventory object throws error.
		{
			inventoryInfo:  nil,
			inventoryLabel: "",
			isError:        true,
		},
		// Pod is not a inventory object.
		{
			inventoryInfo:  pod2Info,
			inventoryLabel: "",
			isError:        true,
		},
		// Retrieves label without preceding/trailing whitespace.
		{
			inventoryInfo:  invInfoLabelWithSpace,
			inventoryLabel: "inventory-label",
			isError:        false,
		},
		{
			inventoryInfo:  invInfo,
			inventoryLabel: testInventoryLabel,
			isError:        false,
		},
	}

	for _, test := range tests {
		actual, err := retrieveInventoryLabel(test.inventoryInfo)
		if test.isError && err == nil {
			t.Errorf("Did not receive expected error.\n")
		}
		if !test.isError {
			if err != nil {
				t.Fatalf("Received unexpected error: %s\n", err)
			}
			if test.inventoryLabel != actual {
				t.Errorf("Expected inventory label (%s), got (%s)\n", test.inventoryLabel, actual)
			}
		}
	}
}

func TestSplitInfos(t *testing.T) {
	tests := map[string]struct {
		infos   []*resource.Info
		inv     *resource.Info
		objs    []*resource.Info
		isError bool
	}{
		"No objects is an error": {
			infos:   []*resource.Info{},
			inv:     nil,
			objs:    []*resource.Info{},
			isError: true,
		},
		"Nil object is an error": {
			infos:   []*resource.Info{nilInfo},
			inv:     nil,
			objs:    []*resource.Info{},
			isError: true,
		},
		"Only inventory object is true": {
			infos:   []*resource.Info{invInfo},
			inv:     invInfo,
			objs:    []*resource.Info{},
			isError: false,
		},
		"Missing inventory object is false": {
			infos:   []*resource.Info{pod1Info},
			inv:     nil,
			objs:    []*resource.Info{pod1Info},
			isError: true,
		},
		"Multiple non-inventory objects is false": {
			infos:   []*resource.Info{pod1Info, pod2Info, pod3Info},
			inv:     nil,
			objs:    []*resource.Info{pod1Info, pod2Info, pod3Info},
			isError: true,
		},
		"Inventory object with multiple others is true": {
			infos:   []*resource.Info{pod1Info, pod2Info, invInfo, pod3Info},
			inv:     invInfo,
			objs:    []*resource.Info{pod1Info, pod2Info, pod3Info},
			isError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			inv, infos, err := SplitInfos(tc.infos)
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error not received")
				}
				return
			}
			if *tc.inv != *inv {
				t.Errorf("expected inventory (%v); got (%v)", *tc.inv, *inv)
			}
			if len(tc.objs) != len(infos) {
				t.Errorf("expected %d objects; got %d", len(tc.objs), len(infos))
			}
		})
	}
}

func TestClearInventoryObject(t *testing.T) {
	pod1 := ignoreErrInfoToObjMeta(pod1Info)
	pod3 := ignoreErrInfoToObjMeta(pod3Info)
	inv := storeObjsInInventory(invInfo, []object.ObjMetadata{pod1, pod3})
	tests := map[string]struct {
		invInfo *resource.Info
		isError bool
	}{
		"Nil info should error": {
			invInfo: nil,
			isError: true,
		},
		"Info with nil Object should error": {
			invInfo: nilInfo,
			isError: true,
		},
		"Single non-inventory object should error": {
			invInfo: pod1Info,
			isError: true,
		},
		"Single inventory object without data should stay cleared": {
			invInfo: invInfo,
			isError: false,
		},
		"Single inventory object with data should be cleared": {
			invInfo: inv,
			isError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invInfo, err := ClearInventoryObj(tc.invInfo)
			if tc.isError {
				if err == nil {
					t.Errorf("Should have produced an error, but returned none.")
				}
			}
			if !tc.isError {
				if err != nil {
					t.Fatalf("Received unexpected error: %s", err)
				}
				wrapped := WrapInventoryObj(invInfo)
				objs, err := wrapped.Load()
				if err != nil {
					t.Fatalf("Received unexpected error: %s", err)
				}
				if len(objs) > 0 {
					t.Errorf("Inventory object inventory not cleared: %#v\n", objs)
				}
			}
		})
	}
}

func TestAddSuffixToName(t *testing.T) {
	tests := []struct {
		info     *resource.Info
		suffix   string
		expected string
		isError  bool
	}{
		// Nil info should return error.
		{
			info:     nil,
			suffix:   "",
			expected: "",
			isError:  true,
		},
		// Empty suffix should return error.
		{
			info:     copyInventoryInfo(),
			suffix:   "",
			expected: "",
			isError:  true,
		},
		// Empty suffix should return error.
		{
			info:     copyInventoryInfo(),
			suffix:   " \t",
			expected: "",
			isError:  true,
		},
		{
			info:     copyInventoryInfo(),
			suffix:   "hashsuffix",
			expected: inventoryObjName + "-hashsuffix",
			isError:  false,
		},
	}

	for _, test := range tests {
		err := addSuffixToName(test.info, test.suffix)
		if test.isError {
			if err == nil {
				t.Errorf("Should have produced an error, but returned none.")
			}
		}
		if !test.isError {
			if err != nil {
				t.Fatalf("Received error when expecting none (%s)\n", err)
			}
			actualName, err := getObjectName(test.info.Object)
			if err != nil {
				t.Fatalf("Error getting object name: %s", err)
			}
			if actualName != test.info.Name {
				t.Errorf("Object name (%s) does not match info name (%s)\n", actualName, test.info.Name)
			}
			if test.expected != actualName {
				t.Errorf("Expected name (%s), got (%s)\n", test.expected, actualName)
			}
		}
	}
}

func TestLegacyInventoryName(t *testing.T) {
	tests := map[string]struct {
		info       *resource.Info
		invName    string // Expected inventory name (if not modified)
		isModified bool   // Should inventory name be changed
		isError    bool   // Should an error be thrown
	}{
		"Nil info is an error": {
			info:       nil,
			isModified: false,
			isError:    true,
		},
		"Nil info.Object is an error": {
			info: &resource.Info{
				Namespace: testNamespace,
				Name:      inventoryObjName,
				Object:    nil,
			},
			invName:    inventoryObjName,
			isModified: false,
			isError:    true,
		},
		"Info name differs from object name should return error": {
			info: &resource.Info{
				Namespace: testNamespace,
				Name:      inventoryObjName,
				Object:    &legacyInvObj,
			},
			invName:    inventoryObjName,
			isModified: false,
			isError:    true,
		},
		"Legacy inventory name gets random suffix": {
			info: &resource.Info{
				Namespace: testNamespace,
				Name:      legacyInvName,
				Object:    &legacyInvObj,
			},
			invName:    legacyInvName,
			isModified: true,
			isError:    false,
		},
		"Non-legacy inventory name does not get modified": {
			info: &resource.Info{
				Namespace: testNamespace,
				Name:      inventoryObjName,
				Object:    &inventoryObj,
			},
			invName:    inventoryObjName,
			isModified: false,
			isError:    false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := fixLegacyInventoryName(tc.info)
			if tc.isError {
				if err == nil {
					t.Fatalf("Should have produced an error, but returned none.")
				}
				return
			}
			if !tc.isError && err != nil {
				t.Fatalf("Received error when expecting none (%s)\n", err)
			}
			actualName, err := getObjectName(tc.info.Object)
			if err != nil {
				t.Fatalf("Error getting object name: %s", err)
			}
			if actualName != tc.info.Name {
				t.Errorf("Object name and info name differ: %s/%s", actualName, tc.info.Name)
			}
			if !tc.isModified {
				if tc.invName != tc.info.Name {
					t.Fatalf("expected non-modified name (%s), got (%s)", tc.invName, tc.info.Name)
				}
				return
			}
			matched, err := regexp.MatchString(`inventory-\d{8}`, actualName)
			if err != nil {
				t.Errorf("unexpected error parsing inventory name: %s", err)
			}
			if !matched {
				t.Errorf("expected inventory name with random suffix, got (%s)", actualName)
			}
		})
	}
}

func getObjectName(obj runtime.Object) (string, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return "", fmt.Errorf("Inventory object is not Unstructured format")
	}
	return u.GetName(), nil
}

func copyInventoryInfo() *resource.Info {
	inventoryObjCopy := inventoryObj.DeepCopy()
	var inventoryInfo = &resource.Info{
		Namespace: testNamespace,
		Name:      inventoryObjName,
		Object:    inventoryObjCopy,
	}
	return inventoryInfo
}

func storeObjsInInventory(inv *resource.Info, objs []object.ObjMetadata) *resource.Info {
	wrapped := WrapInventoryObj(inv)
	_ = wrapped.Store(objs)
	inv, _ = wrapped.GetObject()
	return inv
}

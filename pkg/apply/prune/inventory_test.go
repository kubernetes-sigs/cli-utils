// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	Object:    &pod1,
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
	Object:    &pod2,
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
	Object:    &pod3,
}

var nonUnstructuredInventoryObj = &corev1.ConfigMap{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testNamespace,
		Name:      inventoryObjName,
		Labels: map[string]string{
			common.InventoryLabel: "true",
		},
	},
}

var nonUnstructuredInventoryInfo = &resource.Info{
	Namespace: testNamespace,
	Name:      inventoryObjName,
	Object:    nonUnstructuredInventoryObj,
}

var nilInfo = &resource.Info{
	Namespace: testNamespace,
	Name:      inventoryObjName,
	Object:    nil,
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

func TestRetrieveInventoryLabel(t *testing.T) {
	tests := []struct {
		obj            runtime.Object
		inventoryLabel string
		isError        bool
	}{
		// Nil inventory object throws error.
		{
			obj:            nil,
			inventoryLabel: "",
			isError:        true,
		},
		// Pod is not a inventory object.
		{
			obj:            &pod2,
			inventoryLabel: "",
			isError:        true,
		},
		// Retrieves label without preceding/trailing whitespace.
		{
			obj:            &inventoryObjLabelWithSpace,
			inventoryLabel: "inventory-label",
			isError:        false,
		},
		{
			obj:            &inventoryObj,
			inventoryLabel: testInventoryLabel,
			isError:        false,
		},
	}

	for _, test := range tests {
		actual, err := RetrieveInventoryLabel(test.obj)
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

func TestIsInventoryObject(t *testing.T) {
	tests := []struct {
		obj         runtime.Object
		isInventory bool
	}{
		{
			obj:         nil,
			isInventory: false,
		},
		{
			obj:         &inventoryObj,
			isInventory: true,
		},
		{
			obj:         &pod2,
			isInventory: false,
		},
	}

	for _, test := range tests {
		inventory := IsInventoryObject(test.obj)
		if test.isInventory && !inventory {
			t.Errorf("Inventory object not identified: %#v", test.obj)
		}
		if !test.isInventory && inventory {
			t.Errorf("Non-inventory object identifed as inventory obj: %#v", test.obj)
		}
	}
}

func TestCreateInventoryObject(t *testing.T) {
	testCases := map[string]struct {
		inventoryObjectTemplate *resource.Info
		resources               []*resource.Info

		expectedError     bool
		expectedInventory []*object.ObjMetadata
	}{
		"inventory object template has nil object": {
			inventoryObjectTemplate: nilInfo,
			expectedError:           true,
		},
		"inventory object template is not unstructured": {
			inventoryObjectTemplate: nonUnstructuredInventoryInfo,
			expectedError:           true,
		},
		"no resources": {
			inventoryObjectTemplate: copyInventoryInfo(),
			resources:               []*resource.Info{},
			expectedInventory:       []*object.ObjMetadata{},
		},
		"single resource": {
			inventoryObjectTemplate: copyInventoryInfo(),
			resources:               []*resource.Info{pod1Info},
			expectedInventory: []*object.ObjMetadata{
				{
					Namespace: testNamespace,
					Name:      pod1Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
			},
		},
		"multiple resources": {
			inventoryObjectTemplate: copyInventoryInfo(),
			resources: []*resource.Info{pod1Info, pod2Info,
				pod3Info},
			expectedInventory: []*object.ObjMetadata{
				{
					Namespace: testNamespace,
					Name:      pod1Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod2Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod3Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
			},
		},
		"resource has nil object": {
			inventoryObjectTemplate: copyInventoryInfo(),
			resources:               []*resource.Info{nilInfo},
			expectedError:           true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			invConfigMap := WrapInventoryObj(tc.inventoryObjectTemplate)
			inventoryObj, err := CreateInventoryObj(invConfigMap, tc.resources)
			if tc.expectedError {
				if err == nil {
					t.Errorf("expected error, but didn't get one")
				}
				return
			}

			if !tc.expectedError && err != nil {
				t.Errorf("didn't expect error, but got %v", err)
				return
			}

			accessor, err := meta.Accessor(inventoryObj.Object)
			if err != nil {
				t.Error(err)
			}

			if accessor.GetName() != inventoryObj.Name {
				t.Errorf("expected info and unstructured to have the same name, but they didn't")
			}
			if accessor.GetNamespace() != inventoryObj.Namespace {
				t.Errorf("expected info and unstructured to have the same namespace, but they didn't")
			}

			inv, err := RetrieveObjsFromInventory([]*resource.Info{inventoryObj})
			if err != nil {
				t.Error(err)
			}

			if want, got := len(tc.resources), len(inv); want != got {
				t.Errorf("expected %d resources in inventory, but got %d",
					want, got)
			}
		})
	}
}

func TestFindInventoryObj(t *testing.T) {
	tests := []struct {
		infos  []*resource.Info
		exists bool
		name   string
	}{
		{
			infos:  []*resource.Info{},
			exists: false,
			name:   "",
		},
		{
			infos:  []*resource.Info{nil},
			exists: false,
			name:   "",
		},
		{
			infos:  []*resource.Info{copyInventoryInfo()},
			exists: true,
			name:   inventoryObjName,
		},
		{
			infos:  []*resource.Info{pod1Info},
			exists: false,
			name:   "",
		},
		{
			infos:  []*resource.Info{pod1Info, pod2Info, pod3Info},
			exists: false,
			name:   "",
		},
		{
			infos:  []*resource.Info{pod1Info, pod2Info, copyInventoryInfo(), pod3Info},
			exists: true,
			name:   inventoryObjName,
		},
	}

	for _, test := range tests {
		inventoryObj, found := FindInventoryObj(test.infos)
		if test.exists && !found {
			t.Errorf("Should have found inventory object")
		}
		if !test.exists && found {
			t.Errorf("Inventory object found, but it does not exist: %#v", inventoryObj)
		}
		if test.exists && found && test.name != inventoryObj.Name {
			t.Errorf("Inventory object name does not match: %s/%s", test.name, inventoryObj.Name)
		}
	}
}

func TestAddRetrieveObjsToFromInventory(t *testing.T) {
	tests := []struct {
		infos    []*resource.Info
		expected []*object.ObjMetadata
		isError  bool
	}{
		// No inventory object is an error.
		{
			infos:   []*resource.Info{},
			isError: true,
		},
		// No inventory object is an error.
		{
			infos:   []*resource.Info{pod1Info, pod2Info},
			isError: true,
		},
		// Inventory object without other objects is OK.
		{
			infos:   []*resource.Info{copyInventoryInfo(), nilInfo},
			isError: true,
		},
		{
			infos:   []*resource.Info{nonUnstructuredInventoryInfo},
			isError: true,
		},
		{
			infos:    []*resource.Info{copyInventoryInfo()},
			expected: []*object.ObjMetadata{},
			isError:  false,
		},
		// More than one inventory object is an error.
		{
			infos:    []*resource.Info{copyInventoryInfo(), copyInventoryInfo()},
			expected: []*object.ObjMetadata{},
			isError:  true,
		},
		// More than one inventory object is an error.
		{
			infos:    []*resource.Info{copyInventoryInfo(), pod1Info, copyInventoryInfo()},
			expected: []*object.ObjMetadata{},
			isError:  true,
		},
		// Basic test case: one inventory object, one pod.
		{
			infos: []*resource.Info{copyInventoryInfo(), pod1Info},
			expected: []*object.ObjMetadata{
				{
					Namespace: testNamespace,
					Name:      pod1Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
			},
			isError: false,
		},
		{
			infos: []*resource.Info{pod1Info, copyInventoryInfo()},
			expected: []*object.ObjMetadata{
				{
					Namespace: testNamespace,
					Name:      pod1Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
			},
			isError: false,
		},
		{
			infos: []*resource.Info{pod1Info, pod2Info, copyInventoryInfo(), pod3Info},
			expected: []*object.ObjMetadata{
				{
					Namespace: testNamespace,
					Name:      pod1Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod2Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod3Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
			},
			isError: false,
		},
		{
			infos: []*resource.Info{pod1Info, pod2Info, pod3Info, copyInventoryInfo()},
			expected: []*object.ObjMetadata{
				{
					Namespace: testNamespace,
					Name:      pod1Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod2Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod3Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
			},
			isError: false,
		},
		{
			infos: []*resource.Info{copyInventoryInfo(), pod1Info, pod2Info, pod3Info},
			expected: []*object.ObjMetadata{
				{
					Namespace: testNamespace,
					Name:      pod1Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod2Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
				{
					Namespace: testNamespace,
					Name:      pod3Name,
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Pod",
					},
				},
			},
			isError: false,
		},
	}

	for _, test := range tests {
		err := addObjsToInventory(test.infos)
		if test.isError && err == nil {
			t.Errorf("Should have produced an error, but returned none.")
		}
		if !test.isError {
			if err != nil {
				t.Fatalf("Received error when expecting none (%s)\n", err)
			}
			retrieved, err := RetrieveObjsFromInventory(test.infos)
			if err != nil {
				t.Fatalf("Error retrieving inventory: %s\n", err)
			}
			if len(test.expected) != len(retrieved) {
				t.Errorf("Expected inventory for %d resources, actual %d",
					len(test.expected), len(retrieved))
			}
			for _, expected := range test.expected {
				found := false
				for _, actual := range retrieved {
					if expected.Equals(actual) {
						found = true
						continue
					}
				}
				if !found {
					t.Errorf("Expected inventory (%s) not found", expected)
				}
			}
			// If the inventory object has an inventory, check the
			// inventory object has an inventory hash.
			inventoryInfo, exists := FindInventoryObj(test.infos)
			if exists && len(test.expected) > 0 {
				invHash := retrieveInventoryHash(inventoryInfo)
				if len(invHash) == 0 {
					t.Errorf("Inventory object missing inventory hash")
				}
			}
		}
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
		//t.Errorf("%#v [%s]", test.info, test.suffix)
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

func TestClearInventoryObject(t *testing.T) {
	tests := map[string]struct {
		infos   []*resource.Info
		isError bool
	}{
		"Empty infos should error": {
			infos:   []*resource.Info{},
			isError: true,
		},
		"Non-Unstructured inventory object should error": {
			infos:   []*resource.Info{nonUnstructuredInventoryInfo},
			isError: true,
		},
		"Info with nil Object should error": {
			infos:   []*resource.Info{nilInfo},
			isError: true,
		},
		"Single inventory object should work": {
			infos:   []*resource.Info{copyInventoryInfo()},
			isError: false,
		},
		"Single non-inventory object should error": {
			infos:   []*resource.Info{pod1Info},
			isError: true,
		},
		"Multiple non-inventory objects should error": {
			infos:   []*resource.Info{pod1Info, pod2Info},
			isError: true,
		},
		"Inventory object with single inventory object should work": {
			infos:   []*resource.Info{copyInventoryInfo(), pod1Info},
			isError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := ClearInventoryObj(tc.infos)
			if tc.isError {
				if err == nil {
					t.Errorf("Should have produced an error, but returned none.")
				}
			}
			if !tc.isError {
				if err != nil {
					t.Fatalf("Received unexpected error: %#v", err)
				}
				objMetadata, err := RetrieveObjsFromInventory(tc.infos)
				if err != nil {
					t.Fatalf("Received unexpected error: %#v", err)
				}
				if len(objMetadata) > 0 {
					t.Errorf("Inventory object inventory not cleared: %#v\n", objMetadata)
				}
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

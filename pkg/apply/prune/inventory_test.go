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

var testNamespace = "test-grouping-namespace"
var groupingObjName = "test-grouping-obj"
var pod1Name = "pod-1"
var pod2Name = "pod-2"
var pod3Name = "pod-3"

var testInventoryLabel = "test-app-label"

var groupingObj = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      groupingObjName,
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

var nonUnstructuredGroupingObj = &corev1.ConfigMap{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: testNamespace,
		Name:      groupingObjName,
		Labels: map[string]string{
			common.InventoryLabel: "true",
		},
	},
}

var nonUnstructuredGroupingInfo = &resource.Info{
	Namespace: testNamespace,
	Name:      groupingObjName,
	Object:    nonUnstructuredGroupingObj,
}

var nilInfo = &resource.Info{
	Namespace: testNamespace,
	Name:      groupingObjName,
	Object:    nil,
}

var groupingObjLabelWithSpace = unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      groupingObjName,
			"namespace": testNamespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: "\tgrouping-label ",
			},
		},
	},
}

func TestRetrieveGroupingLabel(t *testing.T) {
	tests := []struct {
		obj           runtime.Object
		groupingLabel string
		isError       bool
	}{
		// Nil grouping object throws error.
		{
			obj:           nil,
			groupingLabel: "",
			isError:       true,
		},
		// Pod is not a grouping object.
		{
			obj:           &pod2,
			groupingLabel: "",
			isError:       true,
		},
		// Retrieves label without preceding/trailing whitespace.
		{
			obj:           &groupingObjLabelWithSpace,
			groupingLabel: "grouping-label",
			isError:       false,
		},
		{
			obj:           &groupingObj,
			groupingLabel: testInventoryLabel,
			isError:       false,
		},
	}

	for _, test := range tests {
		actual, err := retrieveInventoryLabel(test.obj)
		if test.isError && err == nil {
			t.Errorf("Did not receive expected error.\n")
		}
		if !test.isError {
			if err != nil {
				t.Fatalf("Received unexpected error: %s\n", err)
			}
			if test.groupingLabel != actual {
				t.Errorf("Expected grouping label (%s), got (%s)\n", test.groupingLabel, actual)
			}
		}
	}
}

func TestIsInventoryObject(t *testing.T) {
	tests := []struct {
		obj        runtime.Object
		isGrouping bool
	}{
		{
			obj:        nil,
			isGrouping: false,
		},
		{
			obj:        &groupingObj,
			isGrouping: true,
		},
		{
			obj:        &pod2,
			isGrouping: false,
		},
	}

	for _, test := range tests {
		grouping := IsInventoryObject(test.obj)
		if test.isGrouping && !grouping {
			t.Errorf("Inventory object not identified: %#v", test.obj)
		}
		if !test.isGrouping && grouping {
			t.Errorf("Non-inventory object identifed as grouping obj: %#v", test.obj)
		}
	}
}

func TestCreateInventoryObject(t *testing.T) {
	testCases := map[string]struct {
		groupingObjectTemplate *resource.Info
		resources              []*resource.Info

		expectedError     bool
		expectedInventory []*object.ObjMetadata
	}{
		"grouping object template has nil object": {
			groupingObjectTemplate: nilInfo,
			expectedError:          true,
		},
		"grouping object template is not unstructured": {
			groupingObjectTemplate: nonUnstructuredGroupingInfo,
			expectedError:          true,
		},
		"no resources": {
			groupingObjectTemplate: copyGroupingInfo(),
			resources:              []*resource.Info{},
			expectedInventory:      []*object.ObjMetadata{},
		},
		"single resource": {
			groupingObjectTemplate: copyGroupingInfo(),
			resources:              []*resource.Info{pod1Info},
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
			groupingObjectTemplate: copyGroupingInfo(),
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
			groupingObjectTemplate: copyGroupingInfo(),
			resources:              []*resource.Info{nilInfo},
			expectedError:          true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			groupingObj, err := CreateInventoryObj(tc.groupingObjectTemplate,
				tc.resources)

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

			accessor, err := meta.Accessor(groupingObj.Object)
			if err != nil {
				t.Error(err)
			}

			if accessor.GetName() != groupingObj.Name {
				t.Errorf("expected info and unstructured to have the same name, but they didn't")
			}
			if accessor.GetNamespace() != groupingObj.Namespace {
				t.Errorf("expected info and unstructured to have the same namespace, but they didn't")
			}

			inv, err := RetrieveInventoryFromGroupingObj(
				[]*resource.Info{groupingObj})
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
			infos:  []*resource.Info{copyGroupingInfo()},
			exists: true,
			name:   groupingObjName,
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
			infos:  []*resource.Info{pod1Info, pod2Info, copyGroupingInfo(), pod3Info},
			exists: true,
			name:   groupingObjName,
		},
	}

	for _, test := range tests {
		groupingObj, found := FindInventoryObj(test.infos)
		if test.exists && !found {
			t.Errorf("Should have found grouping object")
		}
		if !test.exists && found {
			t.Errorf("Grouping object found, but it does not exist: %#v", groupingObj)
		}
		if test.exists && found && test.name != groupingObj.Name {
			t.Errorf("Grouping object name does not match: %s/%s", test.name, groupingObj.Name)
		}
	}
}

func TestAddRetrieveInventoryToFromGroupingObject(t *testing.T) {
	tests := []struct {
		infos    []*resource.Info
		expected []*object.ObjMetadata
		isError  bool
	}{
		// No grouping object is an error.
		{
			infos:   []*resource.Info{},
			isError: true,
		},
		// No grouping object is an error.
		{
			infos:   []*resource.Info{pod1Info, pod2Info},
			isError: true,
		},
		// Grouping object without other objects is OK.
		{
			infos:   []*resource.Info{copyGroupingInfo(), nilInfo},
			isError: true,
		},
		{
			infos:   []*resource.Info{nonUnstructuredGroupingInfo},
			isError: true,
		},
		{
			infos:    []*resource.Info{copyGroupingInfo()},
			expected: []*object.ObjMetadata{},
			isError:  false,
		},
		// More than one grouping object is an error.
		{
			infos:    []*resource.Info{copyGroupingInfo(), copyGroupingInfo()},
			expected: []*object.ObjMetadata{},
			isError:  true,
		},
		// More than one grouping object is an error.
		{
			infos:    []*resource.Info{copyGroupingInfo(), pod1Info, copyGroupingInfo()},
			expected: []*object.ObjMetadata{},
			isError:  true,
		},
		// Basic test case: one grouping object, one pod.
		{
			infos: []*resource.Info{copyGroupingInfo(), pod1Info},
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
			infos: []*resource.Info{pod1Info, copyGroupingInfo()},
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
			infos: []*resource.Info{pod1Info, pod2Info, copyGroupingInfo(), pod3Info},
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
			infos: []*resource.Info{pod1Info, pod2Info, pod3Info, copyGroupingInfo()},
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
			infos: []*resource.Info{copyGroupingInfo(), pod1Info, pod2Info, pod3Info},
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
		err := AddInventoryToGroupingObj(test.infos)
		if test.isError && err == nil {
			t.Errorf("Should have produced an error, but returned none.")
		}
		if !test.isError {
			if err != nil {
				t.Fatalf("Received error when expecting none (%s)\n", err)
			}
			retrieved, err := RetrieveInventoryFromGroupingObj(test.infos)
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
			// If the grouping object has an inventory, check the
			// grouping object has an inventory hash.
			groupingInfo, exists := FindInventoryObj(test.infos)
			if exists && len(test.expected) > 0 {
				invHash := retrieveInventoryHash(groupingInfo)
				if len(invHash) == 0 {
					t.Errorf("Grouping object missing inventory hash")
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
			info:     copyGroupingInfo(),
			suffix:   "",
			expected: "",
			isError:  true,
		},
		// Empty suffix should return error.
		{
			info:     copyGroupingInfo(),
			suffix:   " \t",
			expected: "",
			isError:  true,
		},
		{
			info:     copyGroupingInfo(),
			suffix:   "hashsuffix",
			expected: groupingObjName + "-hashsuffix",
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
		"Non-Unstructured grouping object should error": {
			infos:   []*resource.Info{nonUnstructuredGroupingInfo},
			isError: true,
		},
		"Info with nil Object should error": {
			infos:   []*resource.Info{nilInfo},
			isError: true,
		},
		"Single grouping object should work": {
			infos:   []*resource.Info{copyGroupingInfo()},
			isError: false,
		},
		"Single non-grouping object should error": {
			infos:   []*resource.Info{pod1Info},
			isError: true,
		},
		"Multiple non-grouping objects should error": {
			infos:   []*resource.Info{pod1Info, pod2Info},
			isError: true,
		},
		"Grouping object with single inventory object should work": {
			infos:   []*resource.Info{copyGroupingInfo(), pod1Info},
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
				objMetadata, err := RetrieveInventoryFromGroupingObj(tc.infos)
				if err != nil {
					t.Fatalf("Received unexpected error: %#v", err)
				}
				if len(objMetadata) > 0 {
					t.Errorf("Grouping object inventory not cleared: %#v\n", objMetadata)
				}
			}
		})
	}
}

func getObjectName(obj runtime.Object) (string, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return "", fmt.Errorf("Grouping object is not Unstructured format")
	}
	return u.GetName(), nil
}

func copyGroupingInfo() *resource.Info {
	groupingObjCopy := groupingObj.DeepCopy()
	var groupingInfo = &resource.Info{
		Namespace: testNamespace,
		Name:      groupingObjName,
		Object:    groupingObjCopy,
	}
	return groupingInfo
}

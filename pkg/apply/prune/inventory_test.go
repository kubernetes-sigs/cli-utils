// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var inventory1 = object.ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-1",
	GroupKind: schema.GroupKind{
		Group: "apps",
		Kind:  "Deployment",
	},
}

var inventory2 = object.ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-2",
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Pod",
	},
}

var inventory3 = object.ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-3",
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Service",
	},
}

var inventory4 = object.ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-4",
	GroupKind: schema.GroupKind{
		Group: "apps",
		Kind:  "DaemonSet",
	},
}

func TestNewInventory(t *testing.T) {
	tests := []struct {
		items        []*object.ObjMetadata
		expectedStr  string
		expectedSize int
	}{
		{
			items:        []*object.ObjMetadata{},
			expectedStr:  "",
			expectedSize: 0,
		},
		{
			items:        []*object.ObjMetadata{&inventory1},
			expectedStr:  "test-namespace_test-inv-1_apps_Deployment",
			expectedSize: 1,
		},
		{
			items:        []*object.ObjMetadata{&inventory1, &inventory2},
			expectedStr:  "test-namespace_test-inv-1_apps_Deployment, test-namespace_test-inv-2__Pod",
			expectedSize: 2,
		},
	}

	for _, test := range tests {
		invSet := NewInventory(test.items)
		actualStr := invSet.String()
		actualSize := invSet.Size()
		if test.expectedStr != actualStr {
			t.Errorf("Expected Inventory (%s), got (%s)\n", test.expectedStr, actualStr)
		}
		if test.expectedSize != actualSize {
			t.Errorf("Expected Inventory size (%d), got (%d)\n", test.expectedSize, actualSize)
		}
		actualItems := invSet.GetItems()
		if len(test.items) != len(actualItems) {
			t.Errorf("Expected num inventory items (%d), got (%d)\n", len(test.items), len(actualItems))
		}
	}
}

func TestInventoryAddItems(t *testing.T) {
	tests := []struct {
		initialItems  []*object.ObjMetadata
		addItems      []*object.ObjMetadata
		expectedItems []*object.ObjMetadata
	}{
		// Adding no items to empty inventory set.
		{
			initialItems:  []*object.ObjMetadata{},
			addItems:      []*object.ObjMetadata{},
			expectedItems: []*object.ObjMetadata{},
		},
		// Adding item to empty inventory set.
		{
			initialItems:  []*object.ObjMetadata{},
			addItems:      []*object.ObjMetadata{&inventory1},
			expectedItems: []*object.ObjMetadata{&inventory1},
		},
		// Adding no items does not change the inventory set
		{
			initialItems:  []*object.ObjMetadata{&inventory1},
			addItems:      []*object.ObjMetadata{},
			expectedItems: []*object.ObjMetadata{&inventory1},
		},
		// Adding an item which alread exists does not increase size.
		{
			initialItems:  []*object.ObjMetadata{&inventory1, &inventory2},
			addItems:      []*object.ObjMetadata{&inventory1},
			expectedItems: []*object.ObjMetadata{&inventory1, &inventory2},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory1, &inventory2},
			addItems:      []*object.ObjMetadata{&inventory3, &inventory4},
			expectedItems: []*object.ObjMetadata{&inventory1, &inventory2, &inventory3, &inventory4},
		},
	}

	for _, test := range tests {
		invSet := NewInventory(test.initialItems)
		invSet.AddItems(test.addItems)
		if len(test.expectedItems) != invSet.Size() {
			t.Errorf("Expected num inventory items (%d), got (%d)\n", len(test.expectedItems), invSet.Size())
		}
	}
}

func TestInventoryDeleteItem(t *testing.T) {
	tests := []struct {
		initialItems  []*object.ObjMetadata
		deleteItem    *object.ObjMetadata
		expected      bool
		expectedItems []*object.ObjMetadata
	}{
		{
			initialItems:  []*object.ObjMetadata{},
			deleteItem:    nil,
			expected:      false,
			expectedItems: []*object.ObjMetadata{},
		},
		{
			initialItems:  []*object.ObjMetadata{},
			deleteItem:    &inventory1,
			expected:      false,
			expectedItems: []*object.ObjMetadata{},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory2},
			deleteItem:    &inventory1,
			expected:      false,
			expectedItems: []*object.ObjMetadata{&inventory2},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory1},
			deleteItem:    &inventory1,
			expected:      true,
			expectedItems: []*object.ObjMetadata{},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory1, &inventory2},
			deleteItem:    &inventory1,
			expected:      true,
			expectedItems: []*object.ObjMetadata{&inventory2},
		},
	}

	for _, test := range tests {
		invSet := NewInventory(test.initialItems)
		actual := invSet.DeleteItem(test.deleteItem)
		if test.expected != actual {
			t.Errorf("Expected return value (%t), got (%t)\n", test.expected, actual)
		}
		if len(test.expectedItems) != invSet.Size() {
			t.Errorf("Expected num inventory items (%d), got (%d)\n", len(test.expectedItems), invSet.Size())
		}
	}
}

func TestInventoryMerge(t *testing.T) {
	tests := []struct {
		set1   []*object.ObjMetadata
		set2   []*object.ObjMetadata
		merged []*object.ObjMetadata
	}{
		{
			set1:   []*object.ObjMetadata{},
			set2:   []*object.ObjMetadata{},
			merged: []*object.ObjMetadata{},
		},
		{
			set1:   []*object.ObjMetadata{},
			set2:   []*object.ObjMetadata{&inventory1},
			merged: []*object.ObjMetadata{&inventory1},
		},
		{
			set1:   []*object.ObjMetadata{&inventory1},
			set2:   []*object.ObjMetadata{},
			merged: []*object.ObjMetadata{&inventory1},
		},
		{
			set1:   []*object.ObjMetadata{&inventory1, &inventory2},
			set2:   []*object.ObjMetadata{&inventory1},
			merged: []*object.ObjMetadata{&inventory1, &inventory2},
		},
		{
			set1:   []*object.ObjMetadata{&inventory1, &inventory2},
			set2:   []*object.ObjMetadata{&inventory1, &inventory2},
			merged: []*object.ObjMetadata{&inventory1, &inventory2},
		},
		{
			set1:   []*object.ObjMetadata{&inventory1, &inventory2},
			set2:   []*object.ObjMetadata{&inventory3, &inventory4},
			merged: []*object.ObjMetadata{&inventory1, &inventory2, &inventory3, &inventory4},
		},
	}

	for _, test := range tests {
		invSet1 := NewInventory(test.set1)
		invSet2 := NewInventory(test.set2)
		expected := NewInventory(test.merged)
		merged, _ := invSet1.Merge(invSet2)
		if expected.Size() != merged.Size() {
			t.Errorf("Expected merged inventory set size (%d), got (%d)\n", expected.Size(), merged.Size())
		}
	}
}

func TestInventorySubtract(t *testing.T) {
	tests := []struct {
		initialItems  []*object.ObjMetadata
		subtractItems []*object.ObjMetadata
		expected      []*object.ObjMetadata
	}{
		{
			initialItems:  []*object.ObjMetadata{},
			subtractItems: []*object.ObjMetadata{},
			expected:      []*object.ObjMetadata{},
		},
		{
			initialItems:  []*object.ObjMetadata{},
			subtractItems: []*object.ObjMetadata{&inventory1},
			expected:      []*object.ObjMetadata{},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory1},
			subtractItems: []*object.ObjMetadata{},
			expected:      []*object.ObjMetadata{&inventory1},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory1, &inventory2},
			subtractItems: []*object.ObjMetadata{&inventory1},
			expected:      []*object.ObjMetadata{&inventory2},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory1, &inventory2},
			subtractItems: []*object.ObjMetadata{&inventory1, &inventory2},
			expected:      []*object.ObjMetadata{},
		},
		{
			initialItems:  []*object.ObjMetadata{&inventory1, &inventory2},
			subtractItems: []*object.ObjMetadata{&inventory3, &inventory4},
			expected:      []*object.ObjMetadata{&inventory1, &inventory2},
		},
	}

	for _, test := range tests {
		invInitialItems := NewInventory(test.initialItems)
		invSubtractItems := NewInventory(test.subtractItems)
		expected := NewInventory(test.expected)
		actual, _ := invInitialItems.Subtract(invSubtractItems)
		if expected.Size() != actual.Size() {
			t.Errorf("Expected subtracted inventory set size (%d), got (%d)\n", expected.Size(), actual.Size())
		}
	}
}

func TestInventoryEquals(t *testing.T) {
	tests := []struct {
		set1    []*object.ObjMetadata
		set2    []*object.ObjMetadata
		isEqual bool
	}{
		{
			set1:    []*object.ObjMetadata{},
			set2:    []*object.ObjMetadata{&inventory1},
			isEqual: false,
		},
		{
			set1:    []*object.ObjMetadata{&inventory1},
			set2:    []*object.ObjMetadata{},
			isEqual: false,
		},
		{
			set1:    []*object.ObjMetadata{&inventory1, &inventory2},
			set2:    []*object.ObjMetadata{&inventory1},
			isEqual: false,
		},
		{
			set1:    []*object.ObjMetadata{&inventory1, &inventory2},
			set2:    []*object.ObjMetadata{&inventory3, &inventory4},
			isEqual: false,
		},
		// Empty sets are equal.
		{
			set1:    []*object.ObjMetadata{},
			set2:    []*object.ObjMetadata{},
			isEqual: true,
		},
		{
			set1:    []*object.ObjMetadata{&inventory1},
			set2:    []*object.ObjMetadata{&inventory1},
			isEqual: true,
		},
		// Ordering of the inventory items does not matter for equality.
		{
			set1:    []*object.ObjMetadata{&inventory1, &inventory2},
			set2:    []*object.ObjMetadata{&inventory2, &inventory1},
			isEqual: true,
		},
	}

	for _, test := range tests {
		invSet1 := NewInventory(test.set1)
		invSet2 := NewInventory(test.set2)
		if !invSet1.Equals(invSet2) && test.isEqual {
			t.Errorf("Expected equal inventory sets; got unequal (%s)/(%s)\n", invSet1, invSet2)
		}
		if invSet1.Equals(invSet2) && !test.isEqual {
			t.Errorf("Expected inequal inventory sets; got equal (%s)/(%s)\n", invSet1, invSet2)
		}
	}
}

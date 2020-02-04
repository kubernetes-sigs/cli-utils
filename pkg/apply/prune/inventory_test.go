// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var inventory1 = ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-1",
	GroupKind: schema.GroupKind{
		Group: "apps",
		Kind:  "Deployment",
	},
}

var inventory2 = ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-2",
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Pod",
	},
}

var inventory3 = ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-3",
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Service",
	},
}

var inventory4 = ObjMetadata{
	Namespace: "test-namespace",
	Name:      "test-inv-4",
	GroupKind: schema.GroupKind{
		Group: "apps",
		Kind:  "DaemonSet",
	},
}

func TestNewInventory(t *testing.T) {
	tests := []struct {
		items        []*ObjMetadata
		expectedStr  string
		expectedSize int
	}{
		{
			items:        []*ObjMetadata{},
			expectedStr:  "",
			expectedSize: 0,
		},
		{
			items:        []*ObjMetadata{&inventory1},
			expectedStr:  "test-namespace_test-inv-1_apps_Deployment",
			expectedSize: 1,
		},
		{
			items:        []*ObjMetadata{&inventory1, &inventory2},
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
		initialItems  []*ObjMetadata
		addItems      []*ObjMetadata
		expectedItems []*ObjMetadata
	}{
		// Adding no items to empty inventory set.
		{
			initialItems:  []*ObjMetadata{},
			addItems:      []*ObjMetadata{},
			expectedItems: []*ObjMetadata{},
		},
		// Adding item to empty inventory set.
		{
			initialItems:  []*ObjMetadata{},
			addItems:      []*ObjMetadata{&inventory1},
			expectedItems: []*ObjMetadata{&inventory1},
		},
		// Adding no items does not change the inventory set
		{
			initialItems:  []*ObjMetadata{&inventory1},
			addItems:      []*ObjMetadata{},
			expectedItems: []*ObjMetadata{&inventory1},
		},
		// Adding an item which alread exists does not increase size.
		{
			initialItems:  []*ObjMetadata{&inventory1, &inventory2},
			addItems:      []*ObjMetadata{&inventory1},
			expectedItems: []*ObjMetadata{&inventory1, &inventory2},
		},
		{
			initialItems:  []*ObjMetadata{&inventory1, &inventory2},
			addItems:      []*ObjMetadata{&inventory3, &inventory4},
			expectedItems: []*ObjMetadata{&inventory1, &inventory2, &inventory3, &inventory4},
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
		initialItems  []*ObjMetadata
		deleteItem    *ObjMetadata
		expected      bool
		expectedItems []*ObjMetadata
	}{
		{
			initialItems:  []*ObjMetadata{},
			deleteItem:    nil,
			expected:      false,
			expectedItems: []*ObjMetadata{},
		},
		{
			initialItems:  []*ObjMetadata{},
			deleteItem:    &inventory1,
			expected:      false,
			expectedItems: []*ObjMetadata{},
		},
		{
			initialItems:  []*ObjMetadata{&inventory2},
			deleteItem:    &inventory1,
			expected:      false,
			expectedItems: []*ObjMetadata{&inventory2},
		},
		{
			initialItems:  []*ObjMetadata{&inventory1},
			deleteItem:    &inventory1,
			expected:      true,
			expectedItems: []*ObjMetadata{},
		},
		{
			initialItems:  []*ObjMetadata{&inventory1, &inventory2},
			deleteItem:    &inventory1,
			expected:      true,
			expectedItems: []*ObjMetadata{&inventory2},
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
		set1   []*ObjMetadata
		set2   []*ObjMetadata
		merged []*ObjMetadata
	}{
		{
			set1:   []*ObjMetadata{},
			set2:   []*ObjMetadata{},
			merged: []*ObjMetadata{},
		},
		{
			set1:   []*ObjMetadata{},
			set2:   []*ObjMetadata{&inventory1},
			merged: []*ObjMetadata{&inventory1},
		},
		{
			set1:   []*ObjMetadata{&inventory1},
			set2:   []*ObjMetadata{},
			merged: []*ObjMetadata{&inventory1},
		},
		{
			set1:   []*ObjMetadata{&inventory1, &inventory2},
			set2:   []*ObjMetadata{&inventory1},
			merged: []*ObjMetadata{&inventory1, &inventory2},
		},
		{
			set1:   []*ObjMetadata{&inventory1, &inventory2},
			set2:   []*ObjMetadata{&inventory1, &inventory2},
			merged: []*ObjMetadata{&inventory1, &inventory2},
		},
		{
			set1:   []*ObjMetadata{&inventory1, &inventory2},
			set2:   []*ObjMetadata{&inventory3, &inventory4},
			merged: []*ObjMetadata{&inventory1, &inventory2, &inventory3, &inventory4},
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
		initialItems  []*ObjMetadata
		subtractItems []*ObjMetadata
		expected      []*ObjMetadata
	}{
		{
			initialItems:  []*ObjMetadata{},
			subtractItems: []*ObjMetadata{},
			expected:      []*ObjMetadata{},
		},
		{
			initialItems:  []*ObjMetadata{},
			subtractItems: []*ObjMetadata{&inventory1},
			expected:      []*ObjMetadata{},
		},
		{
			initialItems:  []*ObjMetadata{&inventory1},
			subtractItems: []*ObjMetadata{},
			expected:      []*ObjMetadata{&inventory1},
		},
		{
			initialItems:  []*ObjMetadata{&inventory1, &inventory2},
			subtractItems: []*ObjMetadata{&inventory1},
			expected:      []*ObjMetadata{&inventory2},
		},
		{
			initialItems:  []*ObjMetadata{&inventory1, &inventory2},
			subtractItems: []*ObjMetadata{&inventory1, &inventory2},
			expected:      []*ObjMetadata{},
		},
		{
			initialItems:  []*ObjMetadata{&inventory1, &inventory2},
			subtractItems: []*ObjMetadata{&inventory3, &inventory4},
			expected:      []*ObjMetadata{&inventory1, &inventory2},
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
		set1    []*ObjMetadata
		set2    []*ObjMetadata
		isEqual bool
	}{
		{
			set1:    []*ObjMetadata{},
			set2:    []*ObjMetadata{&inventory1},
			isEqual: false,
		},
		{
			set1:    []*ObjMetadata{&inventory1},
			set2:    []*ObjMetadata{},
			isEqual: false,
		},
		{
			set1:    []*ObjMetadata{&inventory1, &inventory2},
			set2:    []*ObjMetadata{&inventory1},
			isEqual: false,
		},
		{
			set1:    []*ObjMetadata{&inventory1, &inventory2},
			set2:    []*ObjMetadata{&inventory3, &inventory4},
			isEqual: false,
		},
		// Empty sets are equal.
		{
			set1:    []*ObjMetadata{},
			set2:    []*ObjMetadata{},
			isEqual: true,
		},
		{
			set1:    []*ObjMetadata{&inventory1},
			set2:    []*ObjMetadata{&inventory1},
			isEqual: true,
		},
		// Ordering of the inventory items does not matter for equality.
		{
			set1:    []*ObjMetadata{&inventory1, &inventory2},
			set2:    []*ObjMetadata{&inventory2, &inventory1},
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

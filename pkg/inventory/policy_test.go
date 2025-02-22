// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

type fakeInventoryInfo struct {
	id string
}

func (i *fakeInventoryInfo) Name() string {
	return ""
}

func (i *fakeInventoryInfo) Namespace() string {
	return ""
}

func (i *fakeInventoryInfo) ID() string {
	return i.id
}

func (i *fakeInventoryInfo) Strategy() Strategy {
	return NameStrategy
}

func (i *fakeInventoryInfo) InitialInventory() Inventory {
	return nil
}

func testObjectWithAnnotation(key, val string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "foo",
				"namespace": "ns",
			},
		},
	}
	if key != "" {
		obj.SetAnnotations(map[string]string{
			key: val,
		})
	}
	return obj
}

func TestInventoryIDMatch(t *testing.T) {
	testcases := []struct {
		name     string
		obj      *unstructured.Unstructured
		inv      Info
		expected IDMatchStatus
	}{
		{
			name:     "empty",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			expected: Empty,
		},
		{
			name:     "matched",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			expected: Match,
		},
		{
			name:     "unmatched",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			expected: NoMatch,
		},
	}
	for _, tc := range testcases {
		actual := IDMatch(tc.inv, tc.obj)
		if actual != tc.expected {
			t.Errorf("expected %v, but got %v", tc.expected, actual)
		}
	}
}

func TestCanApply(t *testing.T) {
	testcases := []struct {
		name          string
		obj           *unstructured.Unstructured
		inv           Info
		policy        Policy
		canApply      bool
		expectedError error
	}{
		{
			name:     "empty with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptIfNoInventory,
			canApply: true,
		},
		{
			name:     "empty with AdoptAll",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptAll,
			canApply: true,
		},
		{
			name:     "empty with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyMustMatch,
			canApply: false,
			expectedError: &PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyApply,
				Policy:   PolicyMustMatch,
				Status:   Empty,
			},
		},
		{
			name:     "matched with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   PolicyMustMatch,
			canApply: true,
		},
		{
			name:     "matched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   PolicyAdoptIfNoInventory,
			canApply: true,
		},
		{
			name:     "matched with AloptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   PolicyAdoptAll,
			canApply: true,
		},
		{
			name:     "unmatched with InventoryPolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyMustMatch,
			canApply: false,
			expectedError: &PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyApply,
				Policy:   PolicyMustMatch,
				Status:   NoMatch,
			},
		},
		{
			name:     "unmatched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptIfNoInventory,
			canApply: false,
			expectedError: &PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyApply,
				Policy:   PolicyAdoptIfNoInventory,
				Status:   NoMatch,
			},
		},
		{
			name:     "unmatched with AdoptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptAll,
			canApply: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := CanApply(tc.inv, tc.obj, tc.policy)
			assert.Equal(t, tc.canApply, ok)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

func TestCanPrune(t *testing.T) {
	testcases := []struct {
		name          string
		obj           *unstructured.Unstructured
		inv           Info
		policy        Policy
		canPrune      bool
		expectedError error
	}{
		{
			name:     "empty with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptIfNoInventory,
			canPrune: true,
		},
		{
			name:     "empty with AdoptAll",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptAll,
			canPrune: true,
		},
		{
			name:     "empty with PolicyMustMatch",
			obj:      testObjectWithAnnotation("", ""),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyMustMatch,
			canPrune: false,
			expectedError: &PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyDelete,
				Policy:   PolicyMustMatch,
				Status:   Empty,
			},
		},
		{
			name:     "matched with PolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   PolicyMustMatch,
			canPrune: true,
		},
		{
			name:     "matched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   PolicyAdoptIfNoInventory,
			canPrune: true,
		},
		{
			name:     "matched with AloptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "matched"),
			inv:      &fakeInventoryInfo{id: "matched"},
			policy:   PolicyAdoptAll,
			canPrune: true,
		},
		{
			name:     "unmatched with PolicyMustMatch",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyMustMatch,
			canPrune: false,
			expectedError: &PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyDelete,
				Policy:   PolicyMustMatch,
				Status:   NoMatch,
			},
		},
		{
			name:     "unmatched with AdoptIfNoInventory",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptIfNoInventory,
			canPrune: false,
			expectedError: &PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyDelete,
				Policy:   PolicyAdoptIfNoInventory,
				Status:   NoMatch,
			},
		},
		{
			name:     "unmatched with AdoptAll",
			obj:      testObjectWithAnnotation(OwningInventoryKey, "unmatched"),
			inv:      &fakeInventoryInfo{id: "random-id"},
			policy:   PolicyAdoptAll,
			canPrune: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := CanPrune(tc.inv, tc.obj, tc.policy)
			assert.Equal(t, tc.canPrune, ok)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "inventory-name",
			"namespace": "inventory-namespace",
		},
	},
}

func TestInventoryPolicyPruneFilter(t *testing.T) {
	tests := map[string]struct {
		inventoryID    string
		objInventoryID string
		policy         inventory.Policy
		expectedError  error
	}{
		"inventory and object ids match, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "foo",
			policy:         inventory.PolicyMustMatch,
		},
		"inventory and object ids match and adopt, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "foo",
			policy:         inventory.PolicyAdoptIfNoInventory,
		},
		"inventory and object ids do no match and policy must match, filtered": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.PolicyMustMatch,
			expectedError: &inventory.PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyDelete,
				Policy:   inventory.PolicyMustMatch,
				Status:   inventory.NoMatch,
			},
		},
		"inventory and object ids do no match and adopt if no inventory, filtered": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.PolicyAdoptIfNoInventory,
			expectedError: &inventory.PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyDelete,
				Policy:   inventory.PolicyAdoptIfNoInventory,
				Status:   inventory.NoMatch,
			},
		},
		"inventory and object ids do no match and adopt all, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.PolicyAdoptAll,
		},
		"object id empty and adopt all, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "",
			policy:         inventory.PolicyAdoptAll,
		},
		"object id empty and policy must match, filtered": {
			inventoryID:    "foo",
			objInventoryID: "",
			policy:         inventory.PolicyMustMatch,
			expectedError: &inventory.PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyDelete,
				Policy:   inventory.PolicyMustMatch,
				Status:   inventory.NoMatch,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invIDLabel := map[string]string{
				common.InventoryLabel: tc.inventoryID,
			}
			invObj := inventoryObj.DeepCopy()
			invObj.SetLabels(invIDLabel)
			invInfoObj, err := inventory.ConfigMapToInventoryInfo(invObj)
			require.NoError(t, err)
			filter := InventoryPolicyPruneFilter{
				Inv:       invInfoObj,
				InvPolicy: tc.policy,
			}
			objIDAnnotation := map[string]string{
				"config.k8s.io/owning-inventory": tc.objInventoryID,
			}
			obj := defaultObj.DeepCopy()
			obj.SetAnnotations(objIDAnnotation)
			err = filter.Filter(t.Context(), obj)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

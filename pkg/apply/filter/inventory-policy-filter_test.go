// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "inventory-name",
			"namespace": "inventory-namespace",
		},
	},
}

func TestInventoryPolicyFilter(t *testing.T) {
	tests := map[string]struct {
		inventoryID    string
		objInventoryID string
		policy         inventory.InventoryPolicy
		filtered       bool
	}{
		"inventory and object ids match, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "foo",
			policy:         inventory.InventoryPolicyMustMatch,
			filtered:       false,
		},
		"inventory and object ids match and adopt, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "foo",
			policy:         inventory.AdoptIfNoInventory,
			filtered:       false,
		},
		"inventory and object ids do no match and policy must match, filtered": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.InventoryPolicyMustMatch,
			filtered:       true,
		},
		"inventory and object ids do no match and adopt if no inventory, filtered": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.AdoptIfNoInventory,
			filtered:       true,
		},
		"inventory and object ids do no match and adopt all, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.AdoptAll,
			filtered:       false,
		},
		"object id empty and adopt all, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "",
			policy:         inventory.AdoptAll,
			filtered:       false,
		},
		"object id empty and policy must match, filtered": {
			inventoryID:    "foo",
			objInventoryID: "",
			policy:         inventory.InventoryPolicyMustMatch,
			filtered:       true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invIDLabel := map[string]string{
				common.InventoryLabel: tc.inventoryID,
			}
			invObj := inventoryObj.DeepCopy()
			invObj.SetLabels(invIDLabel)
			filter := InventoryPolicyFilter{
				Inv:       inventory.WrapInventoryInfoObj(invObj),
				InvPolicy: tc.policy,
			}
			objIDAnnotation := map[string]string{
				"config.k8s.io/owning-inventory": tc.objInventoryID,
			}
			obj := defaultObj.DeepCopy()
			obj.SetAnnotations(objIDAnnotation)
			ctx := context.TODO()
			actual, reason, err := filter.Filter(ctx, obj)
			if err != nil {
				t.Fatalf("InventoryPolicyFilter unexpected error (%s)", err)
			}
			if tc.filtered != actual {
				t.Errorf("InventoryPolicyFilter expected filter (%t), got (%t)", tc.filtered, actual)
			}
			if tc.filtered && len(reason) == 0 {
				t.Errorf("InventoryPolicyFilter filtered; expected but missing Reason")
			}
			if !tc.filtered && len(reason) > 0 {
				t.Errorf("InventoryPolicyFilter not filtered; received unexpected Reason: %s", reason)
			}
		})
	}
}

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

var invObjTemplate = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "inventory-name",
			"namespace": "inventory-namespace",
		},
	},
}

func TestInventoryPolicyApplyFilter(t *testing.T) {
	tests := map[string]struct {
		inventoryID    string
		objInventoryID string
		policy         inventory.InventoryPolicy
		filtered       bool
		isError        bool
	}{
		"inventory and object ids match, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "foo",
			policy:         inventory.InventoryPolicyMustMatch,
			filtered:       false,
			isError:        false,
		},
		"inventory and object ids match and adopt, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "foo",
			policy:         inventory.AdoptIfNoInventory,
			filtered:       false,
			isError:        false,
		},
		"inventory and object ids do no match and policy must match, filtered and error": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.InventoryPolicyMustMatch,
			filtered:       true,
			isError:        true,
		},
		"inventory and object ids do no match and adopt if no inventory, filtered and error": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.AdoptIfNoInventory,
			filtered:       true,
			isError:        true,
		},
		"inventory and object ids do no match and adopt all, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.AdoptAll,
			filtered:       false,
			isError:        false,
		},
		"object id empty and adopt all, not filtered": {
			inventoryID:    "foo",
			objInventoryID: "",
			policy:         inventory.AdoptAll,
			filtered:       false,
			isError:        false,
		},
		"object id empty and policy must match, filtered and error": {
			inventoryID:    "foo",
			objInventoryID: "",
			policy:         inventory.InventoryPolicyMustMatch,
			filtered:       true,
			isError:        true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// set owning-inventory annotation
			obj := defaultObj.DeepCopy()
			inventory.SetOwningInventoryAnnotation(obj, tc.objInventoryID)

			invInfo := inventory.InventoryInfoFromObject(inventoryObj)
			invInfo.ID = tc.inventoryID

			filter := InventoryPolicyApplyFilter{
				Client: dynamicfake.NewSimpleDynamicClient(scheme.Scheme, obj),
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...),
				InvInfo:   invInfo,
				InvPolicy: tc.policy,
			}
			actual, reason, err := filter.Filter(obj)
			if tc.isError != (err != nil) {
				t.Fatalf("Expected InventoryPolicyFilter error (%v), got (%v)", tc.isError, (err != nil))
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

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metadatafake "k8s.io/client-go/metadata/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var invObjTemplate = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "inventory-name",
			"namespace": "inventory-namespace",
		},
	},
}

var defaultMetadataObj = &metav1.PartialObjectMetadata{
	ObjectMeta: metav1.ObjectMeta{
		Name:      defaultObj.GetName(),
		Namespace: defaultObj.GetNamespace(),
	},
	TypeMeta: metav1.TypeMeta{
		Kind:       defaultObj.GetKind(),
		APIVersion: defaultObj.GetAPIVersion(),
	},
}

func TestInventoryPolicyApplyFilter(t *testing.T) {
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
		"inventory and object ids do no match and policy must match, filtered and error": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.PolicyMustMatch,
			expectedError: &inventory.PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyApply,
				Policy:   inventory.PolicyMustMatch,
				Status:   inventory.NoMatch,
			},
		},
		"inventory and object ids do no match and adopt if no inventory, filtered and error": {
			inventoryID:    "foo",
			objInventoryID: "bar",
			policy:         inventory.PolicyAdoptIfNoInventory,
			expectedError: &inventory.PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyApply,
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
		"object id empty and policy must match, filtered and error": {
			inventoryID:    "foo",
			objInventoryID: "",
			policy:         inventory.PolicyMustMatch,
			expectedError: &inventory.PolicyPreventedActuationError{
				Strategy: actuation.ActuationStrategyApply,
				Policy:   inventory.PolicyMustMatch,
				Status:   inventory.NoMatch,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			objIDAnnotation := map[string]string{
				"config.k8s.io/owning-inventory": tc.objInventoryID,
			}

			obj := defaultObj.DeepCopy()
			obj.SetAnnotations(objIDAnnotation)
			metadataObj := defaultMetadataObj.DeepCopy()
			metadataObj.SetAnnotations(objIDAnnotation)
			invIDLabel := map[string]string{
				common.InventoryLabel: tc.inventoryID,
			}
			invObj := invObjTemplate.DeepCopy()
			invObj.SetLabels(invIDLabel)
			invInfoObj, err := inventory.ConfigMapToInventoryInfo(invObj)
			require.NoError(t, err)
			filter := InventoryPolicyApplyFilter{
				Client: metadatafake.NewSimpleMetadataClient(scheme.Scheme, metadataObj),
				Mapper: testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...),
				Inv:       invInfoObj,
				InvPolicy: tc.policy,
			}
			err = filter.Filter(t.Context(), obj)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

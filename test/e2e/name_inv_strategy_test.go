// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func applyWithExistingInvTest(ctx context.Context, c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("Apply first set of resources")
	applier := invConfig.ApplierFactoryFunc()
	orgInventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	orgApplyInv := invConfig.InventoryFactoryFunc(inventoryName, namespaceName, orgInventoryID)

	resources := []*unstructured.Unstructured{
		orgApplyInv,
		withNamespace(manifestToUnstructured(deployment1), namespaceName),
	}

	runWithNoErr(applier.Run(ctx, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Verify inventory")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, orgInventoryID, 1)

	By("Apply second set of resources, using same inventory name but different ID")
	secondInventoryID := fmt.Sprintf("%s-%s-2", inventoryName, namespaceName)
	secondApplyInv := invConfig.InventoryFactoryFunc(inventoryName, namespaceName, secondInventoryID)
	resources = []*unstructured.Unstructured{
		secondApplyInv,
		withNamespace(manifestToUnstructured(deployment1), namespaceName),
	}

	err := run(applier.Run(ctx, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Verify that we get the correct error")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("inventory-id of inventory object in cluster doesn't match provided id"))
}

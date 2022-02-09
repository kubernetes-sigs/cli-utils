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
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func deletionPreventionTest(ctx context.Context, c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := inventory.InventoryInfoFromObject(inventoryFactoryFunc(invConfig, inventoryName, namespaceName, inventoryID))

	resources := []*unstructured.Unstructured{
		withNamespace(manifestToUnstructured(deployment1), namespaceName),
		withAnnotation(withNamespace(manifestToUnstructured(pod1), namespaceName), common.OnRemoveAnnotation, common.OnRemoveKeep),
		withAnnotation(withNamespace(manifestToUnstructured(pod2), namespaceName), common.LifecycleDeleteAnnotation, common.PreventDeletion),
	}

	runCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
	}))

	By("Verify deployment created")
	obj := assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(deployment1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID))

	By("Verify pod1 created")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(pod1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID))

	By("Verify pod2 created")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(pod2), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID))

	By("Verify the inventory size is 3")
	invSizeVerifyFunc(ctx, c, invConfig, inventoryName, namespaceName, inventoryID, 3)

	By("Dry-run apply resources")
	resources = []*unstructured.Unstructured{
		withNamespace(manifestToUnstructured(deployment1), namespaceName),
	}

	runCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		DryRunStrategy:   common.DryRunClient,
	}))

	By("Verify deployment still exists and has the config.k8s.io/owning-inventory annotation")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(deployment1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID))

	By("Verify pod1 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(pod1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID))

	By("Verify pod2 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(pod2), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID))

	By("Verify the inventory size is still 3")
	invSizeVerifyFunc(ctx, c, invConfig, inventoryName, namespaceName, inventoryID, 3)

	By("Apply resources")
	resources = []*unstructured.Unstructured{
		withNamespace(manifestToUnstructured(deployment1), namespaceName),
	}

	runCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
	}))

	By("Verify deployment still exists and has the config.k8s.io/owning-inventory annotation")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(deployment1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID))

	By("Verify pod1 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(pod1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(""))

	By("Verify pod2 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = assertUnstructuredExists(ctx, c, withNamespace(manifestToUnstructured(pod2), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(""))

	By("Verify the inventory size is 1")
	invSizeVerifyFunc(ctx, c, invConfig, inventoryName, namespaceName, inventoryID, 1)
}

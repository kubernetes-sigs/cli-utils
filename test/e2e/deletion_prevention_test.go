// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func deletionPreventionTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	resources := []*unstructured.Unstructured{
		e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName),
		e2eutil.WithAnnotation(e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName), common.OnRemoveAnnotation, common.OnRemoveKeep),
		e2eutil.WithAnnotation(e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod2), namespaceName), common.LifecycleDeleteAnnotation, common.PreventDeletion),
	}

	e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
	}))

	By("Verify deployment created")
	obj := e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID()))

	By("Verify pod1 created")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID()))

	By("Verify pod2 created")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod2), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID()))

	By("Verify the inventory size is 3")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 3, 3)

	By("Dry-run apply resources")
	resources = []*unstructured.Unstructured{
		e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName),
	}

	e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		DryRunStrategy:   common.DryRunClient,
	}))

	By("Verify deployment still exists and has the config.k8s.io/owning-inventory annotation")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID()))

	By("Verify pod1 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID()))

	By("Verify pod2 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod2), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID()))

	By("Verify the inventory size is still 3")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 3, 3)

	By("Apply resources")
	resources = []*unstructured.Unstructured{
		e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName),
	}

	e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
	}))

	By("Verify deployment still exists and has the config.k8s.io/owning-inventory annotation")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.ID()))

	By("Verify pod1 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(""))

	By("Verify pod2 still exits and does not have the config.k8s.io/owning-inventory annotation")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod2), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(""))

	By("Verify the inventory size is 1")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 1, 3)
}

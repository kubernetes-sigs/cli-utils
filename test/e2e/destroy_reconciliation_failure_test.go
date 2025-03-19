// Copyright 2020 The Kubernetes Authors.
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
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func destroyReconciliationFailureTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("apply a single resource, which is referenced in the inventory")
	applier := invConfig.ApplierFactoryFunc()

	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)
	inventoryInfo, err := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)
	Expect(err).ToNot(HaveOccurred())

	podObject := e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName)
	podWithFinalizerObject := e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod2), namespaceName)
	// inject an arbitrary finalizer to prevent garbage collection
	podWithFinalizerObject = e2eutil.WithFinalizer(podWithFinalizerObject, "cli-utils/e2e-test")

	resource1 := []*unstructured.Unstructured{
		podObject,
		podWithFinalizerObject,
	}

	_ = e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resource1, apply.ApplierOptions{
		EmitStatusEvents: false,
	}))

	By("Verify all pods created and ready")
	for _, pod := range resource1 {
		result := e2eutil.AssertUnstructuredExists(ctx, c, pod)
		podIP, found, err := object.NestedField(result.Object, "status", "podIP")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness
	}

	By("Verify inventory")
	// The inventory should have both Pods.
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 2, 2)

	// Destroy the resources, with one resource having a finalizer that blocks
	// garbage collection
	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	options := apply.DestroyerOptions{
		InventoryPolicy: inventory.PolicyAdoptIfNoInventory,
		DeleteTimeout:   10 * time.Second, // one pod is expected to be not pruned, so set a short timeout
	}
	// we should be able to run destroy multiple times and continue tracking the
	// object in the inventory
	expectedCounts := []struct {
		specCount   int
		statusCount int
	}{
		{
			specCount:   1, // one object failed to reconcile, so is retained
			statusCount: 2, // status for two objects, one deleted successfully
		},
		{
			specCount:   1,
			statusCount: 1, // only one object in inventory now, still failing to reconcile
		},
		{
			specCount:   1,
			statusCount: 1,
		},
	}
	for _, ec := range expectedCounts {
		_ = e2eutil.RunCollect(destroyer.Run(ctx, inventoryInfo, options))

		By("Verify pod1 is deleted")
		e2eutil.AssertUnstructuredDoesNotExist(ctx, c, podObject)

		By("Verify podWithFinalizerObject is not deleted but has deletion timestamp")
		podWithFinalizerObject = e2eutil.AssertHasDeletionTimestamp(ctx, c, podWithFinalizerObject)

		By("Verify inventory")
		// The inventory should still have the Pod with the finalizer.
		invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, ec.specCount, ec.statusCount)
	}
	// remove the finalizer
	podWithFinalizerObject = e2eutil.WithoutFinalizers(podWithFinalizerObject)
	e2eutil.ApplyUnstructured(ctx, c, podWithFinalizerObject)
	// re-run the destroyer and verify the object is removed from the inventory
	_ = e2eutil.RunCollect(destroyer.Run(ctx, inventoryInfo, options))

	By("Verify pod1 is deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, podObject)

	By("Verify podWithFinalizerObject is deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, podWithFinalizerObject)

	By("Verify inventory")
	// The inventory should be deleted.
	invConfig.InvNotExistsFunc(ctx, c, inventoryName, namespaceName, inventoryID)
}

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
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func applyWithExistingInvTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply first set of resources")
	applier := invConfig.ApplierFactoryFunc()

	firstInventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)
	firstInventoryInfo, err := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, firstInventoryID)
	Expect(err).NotTo(HaveOccurred())

	resources := []*unstructured.Unstructured{
		e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName),
	}

	e2eutil.RunWithNoErr(applier.Run(ctx, firstInventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Verify inventory")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, firstInventoryID, 1, 1)

	By("Apply second set of resources, using same inventory name but different ID")
	secondInventoryID := fmt.Sprintf("%s-%s-2", inventoryName, namespaceName)
	secondInventoryInfo, err := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, secondInventoryID)
	Expect(err).NotTo(HaveOccurred())

	err = e2eutil.Run(applier.Run(ctx, secondInventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Verify that we get the correct error")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("expected inventory object to have inventory-id"))
}

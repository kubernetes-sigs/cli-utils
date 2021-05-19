// Copyright 2020 The Kubernetes Authors.
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
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func applyAndDestroyTest(c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	applyInv := createInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	resources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
	}

	applyCh := applier.Run(context.TODO(), applyInv, resources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	})

	var applierEvents []event.Event
	for e := range applyCh {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
	err := testutil.VerifyEvents([]testutil.ExpEvent{
		{
			EventType: event.InitType,
		},
		{
			EventType: event.ApplyType,
		},
	}, applierEvents)
	Expect(err).ToNot(HaveOccurred())

	By("Verify inventory")
	invConfig.InvSizeVerifyFunc(c, inventoryName, namespaceName, inventoryID, 1)

	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	destroyInv := createInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)
	option := &apply.DestroyerOption{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollectNoErr(destroyer.Run(destroyInv, option))
	err = testutil.VerifyEvents([]testutil.ExpEvent{
		{
			EventType: event.DeleteType,
		},
	}, destroyerEvents)
	Expect(err).ToNot(HaveOccurred())
}

func createInventoryInfo(invConfig InventoryConfig, inventoryName, namespaceName, inventoryID string) inventory.InventoryInfo {
	switch invConfig.InventoryStrategy {
	case inventory.NameStrategy:
		return invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, randomString("inventory-")))
	case inventory.LabelStrategy:
		return invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(randomString("inventory-"), namespaceName, inventoryID))
	default:
		panic(fmt.Errorf("unknown inventory strategy %q", invConfig.InventoryStrategy))
	}
}

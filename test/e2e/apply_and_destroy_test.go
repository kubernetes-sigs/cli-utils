// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func applyAndDestroyTest(c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	resources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
	}

	applyCh := applier.Run(context.TODO(), inv, resources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	})

	var applierEvents []event.Event
	for e := range applyCh {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
	err := verifyEvents([]expEvent{
		{
			eventType: event.InitType,
		},
		{
			eventType: event.ApplyType,
		},
		{
			eventType: event.ApplyType,
		},
		{
			eventType: event.PruneType,
		},
	}, applierEvents)
	Expect(err).ToNot(HaveOccurred())

	By("Verify inventory")
	invConfig.InvSizeVerifyFunc(c, inventoryName, namespaceName, 1)

	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	destroyerEvents := runCollectNoErr(destroyer.Run(inv))
	err = verifyEvents([]expEvent{
		{
			eventType: event.DeleteType,
		},
		{
			eventType: event.DeleteType,
		},
	}, destroyerEvents)
	Expect(err).ToNot(HaveOccurred())
}

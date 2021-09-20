// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func pruneRetrieveErrorTest(c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply a single resource, which is referenced in the inventory")
	applier := invConfig.ApplierFactoryFunc()

	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inv := createInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	resource1 := []*unstructured.Unstructured{
		manifestToUnstructured(pod1),
	}

	ch := applier.Run(context.TODO(), inv, resource1, apply.Options{
		EmitStatusEvents: false,
	})

	var applierEvents []event.Event
	for e := range ch {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
	err := testutil.VerifyEvents([]testutil.ExpEvent{
		{
			// Pod1 is applied
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod1)),
				Operation:  event.Created,
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
		},
	}, applierEvents)
	Expect(err).ToNot(HaveOccurred())

	// Delete the previously applied resource, which is referenced in the inventory.
	By("delete resource, which is referenced in the inventory")
	deleteObj(c, resource1[0])

	By("Verify inventory")
	// The inventory should still have the previously deleted item.
	invConfig.InvSizeVerifyFunc(c, inventoryName, namespaceName, inventoryID, 1)

	By("apply a different resource, and validate the inventory accurately reflects only this object")
	resource2 := []*unstructured.Unstructured{
		manifestToUnstructured(pod2),
	}

	ch = applier.Run(context.TODO(), inv, resource2, apply.Options{
		EmitStatusEvents: false,
	})

	var applierEvents2 []event.Event
	for e := range ch {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents2 = append(applierEvents2, e)
	}
	err = testutil.VerifyEvents([]testutil.ExpEvent{
		{
			// Pod2 is applied
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod2)),
				Operation:  event.Created,
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
		},
	}, applierEvents2)
	Expect(err).ToNot(HaveOccurred())

	By("Verify inventory")
	// The inventory should only have the currently applied item.
	invConfig.InvSizeVerifyFunc(c, inventoryName, namespaceName, inventoryID, 1)

	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	destroyInv := createInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)
	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollectNoErr(destroyer.Run(destroyInv, options))
	err = testutil.VerifyEvents([]testutil.ExpEvent{
		{
			EventType: event.DeleteType,
		},
	}, destroyerEvents)
	Expect(err).ToNot(HaveOccurred())
}

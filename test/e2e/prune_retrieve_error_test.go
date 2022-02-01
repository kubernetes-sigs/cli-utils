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

func pruneRetrieveErrorTest(ctx context.Context, c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply a single resource, which is referenced in the inventory")
	applier := invConfig.ApplierFactoryFunc()

	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inv := createInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	pod1Obj := withNamespace(manifestToUnstructured(pod1), namespaceName)
	resource1 := []*unstructured.Unstructured{
		pod1Obj,
	}

	applierEvents := runCollect(applier.Run(ctx, inv, resource1, apply.ApplierOptions{
		EmitStatusEvents: false,
	}))

	expEvents := []testutil.ExpEvent{
		{
			// InitTask
			EventType: event.InitType,
			InitEvent: &testutil.ExpInitEvent{},
		},
		{
			// InvAddTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-add-0",
				Type:      event.Started,
			},
		},
		{
			// InvAddTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-add-0",
				Type:      event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-0",
				Type:      event.Started,
			},
		},
		{
			// Create Pod1
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-0",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Started,
			},
		},
		{
			// Pod1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// Pod1 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Finished,
			},
		},
		{
			// InvSetTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-set-0",
				Type:      event.Started,
			},
		},
		{
			// InvSetTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-set-0",
				Type:      event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(applierEvents)).To(testutil.Equal(expEvents))

	By("Verify pod1 created and ready")
	result := assertUnstructuredExists(ctx, c, pod1Obj)
	podIP, found, err := object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	// Delete the previously applied resource, which is referenced in the inventory.
	By("delete resource, which is referenced in the inventory")
	deleteUnstructuredAndWait(ctx, c, pod1Obj)

	By("Verify inventory")
	// The inventory should still have the previously deleted item.
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 1)

	By("apply a different resource, and validate the inventory accurately reflects only this object")
	pod2Obj := withNamespace(manifestToUnstructured(pod2), namespaceName)
	resource2 := []*unstructured.Unstructured{
		pod2Obj,
	}

	applierEvents2 := runCollect(applier.Run(ctx, inv, resource2, apply.ApplierOptions{
		EmitStatusEvents: false,
	}))

	expEvents2 := []testutil.ExpEvent{
		{
			// InitTask
			EventType: event.InitType,
			InitEvent: &testutil.ExpInitEvent{},
		},
		{
			// InvAddTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-add-0",
				Type:      event.Started,
			},
		},
		{
			// InvAddTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-add-0",
				Type:      event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-0",
				Type:      event.Started,
			},
		},
		{
			// Create pod2
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-0",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Started,
			},
		},
		{
			// Pod2 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// Pod2 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Finished,
			},
		},
		// Don't prune pod1, it should already be deleted.
		{
			// InvSetTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-set-0",
				Type:      event.Started,
			},
		},
		{
			// InvSetTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-set-0",
				Type:      event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(applierEvents2)).To(testutil.Equal(expEvents2))

	By("Verify pod2 created and ready")
	result = assertUnstructuredExists(ctx, c, pod2Obj)
	podIP, found, err = object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	By("Verify pod1 still deleted")
	assertUnstructuredDoesNotExist(ctx, c, pod1Obj)

	By("Verify inventory")
	// The inventory should only have the currently applied item.
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 1)

	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollect(destroyer.Run(ctx, inv, options))

	expEvents3 := []testutil.ExpEvent{
		{
			// InitTask
			EventType: event.InitType,
			InitEvent: &testutil.ExpInitEvent{},
		},
		{
			// PruneTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-0",
				Type:      event.Started,
			},
		},
		{
			// Delete pod2
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-0",
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-0",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Started,
			},
		},
		{
			// Pod2 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// Pod2 confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Finished,
			},
		},
		{
			// DeleteInvTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "delete-inventory-0",
				Type:      event.Started,
			},
		},
		{
			// DeleteInvTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "delete-inventory-0",
				Type:      event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(destroyerEvents)).To(testutil.Equal(expEvents3))

	By("Verify pod1 is deleted")
	assertUnstructuredDoesNotExist(ctx, c, pod1Obj)

	By("Verify pod2 is deleted")
	assertUnstructuredDoesNotExist(ctx, c, pod2Obj)
}

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
		withNamespace(manifestToUnstructured(pod1), namespaceName),
	}

	ch := applier.Run(context.TODO(), inv, resource1, apply.Options{
		EmitStatusEvents: false,
	})

	var applierEvents []event.Event
	for e := range ch {
		applierEvents = append(applierEvents, e)
	}
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
				Action: event.InventoryAction,
				Name:   "inventory-add-0",
				Type:   event.Started,
			},
		},
		{
			// InvAddTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "inventory-add-0",
				Type:   event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-0",
				Type:   event.Started,
			},
		},
		{
			// Create deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(pod1), namespaceName)),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-0",
				Type:   event.Finished,
			},
		},
		// TODO: Why no waiting???
		// {
		// 	// WaitTask start
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Started,
		// 	},
		// },
		// {
		// 	// WaitTask finished
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Finished,
		// 	},
		// },
		{
			// InvSetTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "inventory-set-0",
				Type:   event.Started,
			},
		},
		{
			// InvSetTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "inventory-set-0",
				Type:   event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(applierEvents)).To(testutil.Equal(expEvents))

	By("Verify pod1 created")
	assertUnstructuredExists(c, withNamespace(manifestToUnstructured(pod1), namespaceName))

	// Delete the previously applied resource, which is referenced in the inventory.
	By("delete resource, which is referenced in the inventory")
	deleteUnstructuredAndWait(c, withNamespace(manifestToUnstructured(pod1), namespaceName))

	By("Verify inventory")
	// The inventory should still have the previously deleted item.
	invConfig.InvSizeVerifyFunc(c, inventoryName, namespaceName, inventoryID, 1)

	By("apply a different resource, and validate the inventory accurately reflects only this object")
	resource2 := []*unstructured.Unstructured{
		withNamespace(manifestToUnstructured(pod2), namespaceName),
	}

	ch = applier.Run(context.TODO(), inv, resource2, apply.Options{
		EmitStatusEvents: false,
	})

	var applierEvents2 []event.Event
	for e := range ch {
		applierEvents2 = append(applierEvents2, e)
	}
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
				Action: event.InventoryAction,
				Name:   "inventory-add-0",
				Type:   event.Started,
			},
		},
		{
			// InvAddTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "inventory-add-0",
				Type:   event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-0",
				Type:   event.Started,
			},
		},
		{
			// Create pod2
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(pod2), namespaceName)),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-0",
				Type:   event.Finished,
			},
		},
		// Don't prune pod1, it should already be deleted.
		// TODO: Why is waiting skipped on create?
		// {
		// 	// WaitTask start
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Started,
		// 	},
		// },
		// {
		// 	// WaitTask finished
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Finished,
		// 	},
		// },
		{
			// InvSetTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "inventory-set-0",
				Type:   event.Started,
			},
		},
		{
			// InvSetTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "inventory-set-0",
				Type:   event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(applierEvents2)).To(testutil.Equal(expEvents2))

	By("Wait for pod2 to be created")
	// TODO: change behavior so the user doesn't need to code their own wait
	waitForCreation(c, withNamespace(manifestToUnstructured(pod2), namespaceName))

	By("Verify pod1 still deleted")
	assertUnstructuredDoesNotExist(c, withNamespace(manifestToUnstructured(pod1), namespaceName))

	By("Verify inventory")
	// The inventory should only have the currently applied item.
	invConfig.InvSizeVerifyFunc(c, inventoryName, namespaceName, inventoryID, 1)

	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollect(destroyer.Run(inv, options))

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
				Action: event.DeleteAction,
				Name:   "prune-0",
				Type:   event.Started,
			},
		},
		{
			// Delete pod2
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				// TODO: this delete is flakey (sometimes skipped), because there's no WaitTask after creation
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(pod2), namespaceName)),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.DeleteAction,
				Name:   "prune-0",
				Type:   event.Finished,
			},
		},
		// TODO: Why is waiting skipped on destroy?
		// {
		// 	// WaitTask start
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Started,
		// 	},
		// },
		// {
		// 	// WaitTask finished
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Finished,
		// 	},
		// },
		{
			// DeleteInvTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "delete-inventory-0",
				Type:   event.Started,
			},
		},
		{
			// DeleteInvTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "delete-inventory-0",
				Type:   event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(destroyerEvents)).To(testutil.Equal(expEvents3))

	By("Verify pod1 is deleted")
	assertUnstructuredDoesNotExist(c, withNamespace(manifestToUnstructured(pod1), namespaceName))

	By("Wait for pod2 to be deleted")
	// TODO: change behavior so the user doesn't need to code their own wait
	waitForDeletion(c, withNamespace(manifestToUnstructured(pod2), namespaceName))
}

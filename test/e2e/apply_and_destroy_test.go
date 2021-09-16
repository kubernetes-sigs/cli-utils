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
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func applyAndDestroyTest(c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := createInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	resources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
	}

	applyCh := applier.Run(context.TODO(), inventoryInfo, resources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	})

	var applierEvents []event.Event
	for e := range applyCh {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
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
				Identifier: object.UnstructuredToObjMetaOrDie(deploymentManifest(namespaceName)),
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
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-0",
				Type:   event.Started,
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-0",
				Type:   event.Finished,
			},
		},
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
	received := testutil.EventsToExpEvents(applierEvents)

	// handle required async NotFound StatusEvents
	expected := testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetaOrDie(deploymentManifest(namespaceName)),
			Status:     status.NotFoundStatus,
			Error:      nil,
		},
	}
	received, matches := testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.NotFoundStatus)

	// handle optional async InProgress StatusEvents
	expected = testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetaOrDie(deploymentManifest(namespaceName)),
			Status:     status.InProgressStatus,
			Error:      nil,
		},
	}
	received, _ = testutil.RemoveEqualEvents(received, expected)

	// handle required async Current StatusEvents
	expected = testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetaOrDie(deploymentManifest(namespaceName)),
			Status:     status.CurrentStatus,
			Error:      nil,
		},
	}
	received, matches = testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	Expect(received).To(testutil.Equal(expEvents))

	By("Verify inventory")
	invConfig.InvSizeVerifyFunc(c, inventoryName, namespaceName, inventoryID, 1)

	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollectNoErr(destroyer.Run(inventoryInfo, options))

	expEvents = []testutil.ExpEvent{
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
			// Delete deployment
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(deploymentManifest(namespaceName)),
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

	Expect(testutil.EventsToExpEvents(destroyerEvents)).To(testutil.Equal(expEvents))
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

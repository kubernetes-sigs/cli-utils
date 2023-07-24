// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func applyAndDestroyTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	deployment1Obj := e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName)
	resources := []*unstructured.Unstructured{
		deployment1Obj,
	}

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
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
			// Create deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
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
			// Deployment reconcile Pending .
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			},
		},
		{
			// Deployment confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
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
	receivedEvents := testutil.EventsToExpEvents(applierEvents)

	// handle optional async NotFound StatusEvents
	expected := testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.NotFoundStatus,
			Error:      nil,
		},
	}
	receivedEvents, _ = testutil.RemoveEqualEvents(receivedEvents, expected)

	// handle optional async InProgress StatusEvents
	expected = testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.InProgressStatus,
			Error:      nil,
		},
	}
	receivedEvents, _ = testutil.RemoveEqualEvents(receivedEvents, expected)

	// handle required async Current StatusEvents
	expected = testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.CurrentStatus,
			Error:      nil,
		},
	}
	receivedEvents, matches := testutil.RemoveEqualEvents(receivedEvents, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	expEvents, receivedEvents = e2eutil.FilterOptionalEvents(expEvents, receivedEvents)

	Expect(receivedEvents).To(testutil.Equal(expEvents))

	By("Verify deployment created")
	e2eutil.AssertUnstructuredExists(ctx, c, deployment1Obj)

	By("Verify inventory")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 1, 1)

	By("Destroy resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	options := apply.DestroyerOptions{InventoryPolicy: inventory.PolicyAdoptIfNoInventory}
	destroyerEvents := e2eutil.RunCollect(destroyer.Run(ctx, inventoryInfo, options))

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
				Action:    event.DeleteAction,
				GroupName: "prune-0",
				Type:      event.Started,
			},
		},
		{
			// Delete deployment
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-0",
				Status:     event.DeleteSuccessful,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
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
			// Deployment reconcile Pending .
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			},
		},
		{
			// Deployment confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
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
				GroupName: "inventory-delete-or-update-0",
				Type:      event.Started,
			},
		},
		{
			// DeleteInvTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-delete-or-update-0",
				Type:      event.Finished,
			},
		},
	}
	receivedEvents = testutil.EventsToExpEvents(destroyerEvents)

	expEvents, receivedEvents = e2eutil.FilterOptionalEvents(expEvents, receivedEvents)

	Expect(receivedEvents).To(testutil.Equal(expEvents))

	By("Verify deployment deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, deployment1Obj)
}

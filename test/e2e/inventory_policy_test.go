// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"errors"
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

func inventoryPolicyMustMatchTest(ctx context.Context, c client.Client, invConfig InventoryConfig, namespaceName string) {
	By("Apply first set of resources")
	applier := invConfig.ApplierFactoryFunc()

	firstInvName := randomString("first-inv-")
	firstInv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(firstInvName, namespaceName, firstInvName))
	deployment1Obj := withNamespace(manifestToUnstructured(deployment1), namespaceName)
	firstResources := []*unstructured.Unstructured{
		deployment1Obj,
	}

	runWithNoErr(applier.Run(ctx, firstInv, firstResources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Apply second set of resources")
	secondInvName := randomString("second-inv-")
	secondInv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(secondInvName, namespaceName, secondInvName))
	deployment1Obj = withNamespace(manifestToUnstructured(deployment1), namespaceName)
	secondResources := []*unstructured.Unstructured{
		withReplicas(deployment1Obj, 6),
	}

	applierEvents := runCollect(applier.Run(ctx, secondInv, secondResources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		InventoryPolicy:  inventory.PolicyMustMatch,
	}))

	By("Verify the events")
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
			// ApplyTask error: resource managed by another inventory
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
				Error: testutil.EqualErrorType(
					inventory.NewInventoryOverlapError(errors.New("test")),
				),
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
			// Wait skipped because apply failed
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcileSkipped,
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
	received := testutil.EventsToExpEvents(applierEvents)

	// handle optional async InProgress StatusEvents
	expected := testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.InProgressStatus,
			Error:      nil,
		},
	}
	received, _ = testutil.RemoveEqualEvents(received, expected)

	// handle required async Current StatusEvents
	expected = testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.CurrentStatus,
			Error:      nil,
		},
	}
	received, matches := testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	Expect(received).To(testutil.Equal(expEvents))

	By("Verify resource wasn't updated")
	result := assertUnstructuredExists(ctx, c, deployment1Obj)
	replicas, found, err := object.NestedField(result.Object, "spec", "replicas")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(replicas).To(Equal(int64(4)))

	invConfig.InvCountVerifyFunc(ctx, c, namespaceName, 2)
}

func inventoryPolicyAdoptIfNoInventoryTest(ctx context.Context, c client.Client, invConfig InventoryConfig, namespaceName string) {
	By("Create unmanaged resource")
	deployment1Obj := withNamespace(manifestToUnstructured(deployment1), namespaceName)
	createUnstructuredAndWait(ctx, c, deployment1Obj)

	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()

	invName := randomString("test-inv-")
	inv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(invName, namespaceName, invName))
	deployment1Obj = withNamespace(manifestToUnstructured(deployment1), namespaceName)
	resources := []*unstructured.Unstructured{
		withReplicas(deployment1Obj, 6),
	}

	applierEvents := runCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		InventoryPolicy:  inventory.PolicyAdoptIfNoInventory,
	}))

	By("Verify the events")
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
			// Apply deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Configured,
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
			// Deployment reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			},
		},
		{
			// Deployment confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
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

	// handle optional async InProgress StatusEvents
	received := testutil.EventsToExpEvents(applierEvents)
	expected := testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.InProgressStatus,
			Error:      nil,
		},
	}
	received, _ = testutil.RemoveEqualEvents(received, expected)

	// handle required async Current StatusEvents
	expected = testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.CurrentStatus,
			Error:      nil,
		},
	}
	received, matches := testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	Expect(received).To(testutil.Equal(expEvents))

	By("Verify resource was updated and added to inventory")
	result := assertUnstructuredExists(ctx, c, deployment1Obj)

	replicas, found, err := object.NestedField(result.Object, "spec", "replicas")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(replicas).To(Equal(int64(6)))

	value, found, err := object.NestedField(result.Object, "metadata", "annotations", "config.k8s.io/owning-inventory")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(value).To(Equal(invName))

	invConfig.InvCountVerifyFunc(ctx, c, namespaceName, 1)
	invConfig.InvSizeVerifyFunc(ctx, c, invName, namespaceName, invName, 1, 1)
}

func inventoryPolicyAdoptAllTest(ctx context.Context, c client.Client, invConfig InventoryConfig, namespaceName string) {
	By("Apply an initial set of resources")
	applier := invConfig.ApplierFactoryFunc()

	firstInvName := randomString("first-inv-")
	firstInv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(firstInvName, namespaceName, firstInvName))
	deployment1Obj := withNamespace(manifestToUnstructured(deployment1), namespaceName)
	firstResources := []*unstructured.Unstructured{
		deployment1Obj,
	}

	runWithNoErr(applier.Run(ctx, firstInv, firstResources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Apply resources")
	secondInvName := randomString("test-inv-")
	secondInv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(secondInvName, namespaceName, secondInvName))
	deployment1Obj = withNamespace(manifestToUnstructured(deployment1), namespaceName)
	secondResources := []*unstructured.Unstructured{
		withReplicas(deployment1Obj, 6),
	}

	applierEvents := runCollect(applier.Run(ctx, secondInv, secondResources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		InventoryPolicy:  inventory.PolicyAdoptAll,
	}))

	By("Verify the events")
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
			// Apply deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Configured,
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
			// Deployment reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			},
		},
		{
			// Deployment confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
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

	// handle optional async InProgress StatusEvents
	received := testutil.EventsToExpEvents(applierEvents)
	expected := testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.InProgressStatus,
			Error:      nil,
		},
	}
	received, _ = testutil.RemoveEqualEvents(received, expected)

	// handle required async Current StatusEvents
	expected = testutil.ExpEvent{
		EventType: event.StatusType,
		StatusEvent: &testutil.ExpStatusEvent{
			Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			Status:     status.CurrentStatus,
			Error:      nil,
		},
	}
	received, matches := testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	Expect(received).To(testutil.Equal(expEvents))

	By("Verify resource was updated and added to inventory")
	result := assertUnstructuredExists(ctx, c, deployment1Obj)

	replicas, found, err := object.NestedField(result.Object, "spec", "replicas")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(replicas).To(Equal(int64(6)))

	value, found, err := object.NestedField(result.Object, "metadata", "annotations", "config.k8s.io/owning-inventory")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(value).To(Equal(secondInvName))

	invConfig.InvCountVerifyFunc(ctx, c, namespaceName, 2)
}

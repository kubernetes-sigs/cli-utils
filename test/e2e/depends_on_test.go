// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func dependsOnTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("apply resources in order based on depends-on annotation")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(inventoryName, namespaceName, "test"))

	namespace1Name := fmt.Sprintf("%s-ns1", namespaceName)
	namespace1Obj := e2eutil.UnstructuredNamespace(namespace1Name)

	namespace2Name := fmt.Sprintf("%s-ns2", namespaceName)
	namespace2Obj := e2eutil.UnstructuredNamespace(namespace2Name)

	pod1Obj := e2eutil.ManifestToUnstructured(pod1)
	pod1Obj = e2eutil.WithNamespace(pod1Obj, namespace1Name)
	pod1Obj = e2eutil.WithDependsOn(pod1Obj, fmt.Sprintf("/namespaces/%s/Pod/pod3", namespace1Name))

	pod2Obj := e2eutil.ManifestToUnstructured(pod2)
	pod2Obj = e2eutil.WithNamespace(pod2Obj, namespace2Name)

	pod3Obj := e2eutil.ManifestToUnstructured(pod3)
	pod3Obj = e2eutil.WithNamespace(pod3Obj, namespace1Name)
	pod3Obj = e2eutil.WithDependsOn(pod3Obj, fmt.Sprintf("/namespaces/%s/Pod/pod2", namespace2Name))

	// Dependency order: pod1 -> pod3 -> pod2
	// Apply order: pod2, pod3, pod1
	resources := []*unstructured.Unstructured{
		namespace1Obj,
		namespace2Obj,
		pod1Obj,
		pod2Obj,
		pod3Obj,
	}

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
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
			// Apply Namespace1 first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace1Obj),
			},
		},
		{
			// Apply Namespace2 first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace2Obj),
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
			// Namespace1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(namespace1Obj),
			},
		},
		{
			// Namespace2 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(namespace2Obj),
			},
		},
		{
			// Namespace1 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace1Obj),
			},
		},
		{
			// Namespace2 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace2Obj),
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
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-1",
				Type:      event.Started,
			},
		},
		{
			// Apply Pod2 first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-1",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-1",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-1",
				Type:      event.Started,
			},
		},
		{
			// Pod2 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// Pod2 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-1",
				Type:      event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-2",
				Type:      event.Started,
			},
		},
		{
			// Apply Pod3 second
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-2",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod3Obj),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-2",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-2",
				Type:      event.Started,
			},
		},
		{
			// Pod3 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-2",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod3Obj),
			},
		},
		{
			// Pod3 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-2",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod3Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-2",
				Type:      event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-3",
				Type:      event.Started,
			},
		},
		{
			// Apply Pod1 third
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-3",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-3",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-3",
				Type:      event.Started,
			},
		},
		{
			// Pod1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-3",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// Pod1 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-3",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-3",
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

	expEvents, receivedEvents = e2eutil.FilterOptionalEvents(expEvents, receivedEvents)

	// sort to compensate for wait task reconcile ordering variations
	testutil.SortExpEvents(receivedEvents)

	Expect(receivedEvents).To(testutil.Equal(expEvents))

	By("verify namespace1 created")
	e2eutil.AssertUnstructuredExists(ctx, c, namespace1Obj)

	By("verify namespace2 created")
	e2eutil.AssertUnstructuredExists(ctx, c, namespace2Obj)

	By("verify pod1 created and ready")
	result := e2eutil.AssertUnstructuredExists(ctx, c, pod1Obj)
	podIP, found, err := object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	By("verify pod2 created and ready")
	result = e2eutil.AssertUnstructuredExists(ctx, c, pod2Obj)
	podIP, found, err = object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	By("verify pod3 created and ready")
	result = e2eutil.AssertUnstructuredExists(ctx, c, pod3Obj)
	podIP, found, err = object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	By("destroy resources in opposite order")
	destroyer := invConfig.DestroyerFactoryFunc()
	options := apply.DestroyerOptions{InventoryPolicy: inventory.PolicyAdoptIfNoInventory}
	destroyerEvents := e2eutil.RunCollect(destroyer.Run(ctx, inv, options))

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
			// Delete pod1 first
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-0",
				Status:     event.DeleteSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
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
			// Pod1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// Pod1 confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSuccessful,
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
			// PruneTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-1",
				Type:      event.Started,
			},
		},
		{
			// Delete pod3 second
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-1",
				Status:     event.DeleteSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod3Obj),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-1",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-1",
				Type:      event.Started,
			},
		},
		{
			// Pod3 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod3Obj),
			},
		},
		{
			// Pod3 confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod3Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-1",
				Type:      event.Finished,
			},
		},
		{
			// PruneTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-2",
				Type:      event.Started,
			},
		},
		{
			// Delete pod2 third
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-2",
				Status:     event.DeleteSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-2",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-2",
				Type:      event.Started,
			},
		},
		{
			// Pod2 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-2",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// Pod2 confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-2",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-2",
				Type:      event.Finished,
			},
		},
		{
			// PruneTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-3",
				Type:      event.Started,
			},
		},
		{
			// Delete Namespace1 last
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-3",
				Status:     event.DeleteSuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace2Obj),
			},
		},
		{
			// Delete Namespace2 last
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-3",
				Status:     event.DeleteSuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace1Obj),
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-3",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-3",
				Type:      event.Started,
			},
		},
		{
			// Namespace1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-3",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(namespace1Obj),
			},
		},
		{
			// Namespace2 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-3",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(namespace2Obj),
			},
		},
		{
			// Namespace1 confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-3",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace1Obj),
			},
		},
		{
			// Namespace2 confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-3",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(namespace2Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-3",
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

	// sort to handle objects reconciling in random order
	testutil.SortExpEvents(receivedEvents)

	Expect(receivedEvents).To(testutil.Equal(expEvents))

	By("verify pod1 deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, pod1Obj)

	By("verify pod2 deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, pod2Obj)

	By("verify pod3 deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, pod3Obj)

	By("verify namespace1 deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, namespace1Obj)

	By("verify namespace2 deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, namespace2Obj)
}

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

// High priority use cases:
// - IAMPolicyMember .spec.member injected with apply-time-mutation using a name
//   that contains Project .status.number
//   https://github.com/GoogleCloudPlatform/k8s-config-connector/issues/340
// - Service .spec.loadBalancerIP injected with apply-time-mutation from
//   ComputeAddress .spec.address
//   https://github.com/GoogleCloudPlatform/k8s-config-connector/issues/334
//
// However, since both of these use Config Connector resources, which use CRDs
// that are copyright to Google, we can't use them as e2e tests here. Instead,
// we test a toy example with a pod-a depending on pod-b, injecting the ip and
// port from pod-b into an environment variable of pod-a.

//nolint:dupl // expEvents similar to CRD tests
func mutationTest(ctx context.Context, c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply resources in order with substitutions based on apply-time-mutation annotation")
	applier := invConfig.ApplierFactoryFunc()

	inv := inventory.InventoryInfoFromObject(inventoryFactoryFunc(invConfig, inventoryName, namespaceName, "test"))

	fields := struct{ Namespace string }{Namespace: namespaceName}
	podAObj := templateToUnstructured(podATemplate, fields)
	podBObj := templateToUnstructured(podBTemplate, fields)

	// Dependency order: podA -> podB
	// Apply order: podB, podA
	resources := []*unstructured.Unstructured{
		podAObj,
		podBObj,
	}

	applierEvents := runCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
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
			// Apply PodB first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(podBObj),
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
			// PodB reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(podBObj),
			},
		},
		{
			// PodB confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(podBObj),
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
			// Apply PodA second
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-1",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(podAObj),
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
			// PodA reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(podAObj),
			},
		},
		{
			// PodA confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(podAObj),
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

	By("verify podB is created and ready")
	result := assertUnstructuredExists(ctx, c, podBObj)

	podIP, found, err := object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	containerPort, found, err := object.NestedField(result.Object, "spec", "containers", 0, "ports", 0, "containerPort")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(containerPort).To(Equal(int64(80)))

	host := fmt.Sprintf("%s:%d", podIP, containerPort)

	By("verify podA is mutated, created, and ready")
	result = assertUnstructuredExists(ctx, c, podAObj)

	podIP, found, err = object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	envValue, found, err := object.NestedField(result.Object, "spec", "containers", 0, "env", 0, "value")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(envValue).To(Equal(host))

	By("destroy resources in opposite order")
	destroyer := invConfig.DestroyerFactoryFunc()
	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollect(destroyer.Run(ctx, inv, options))

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
			// Delete PodA first
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-0",
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetadata(podAObj),
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
			// PodA reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(podAObj),
			},
		},
		{
			// PodA confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(podAObj),
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
			// Delete PodB second
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-1",
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetadata(podBObj),
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
			// PodB reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(podBObj),
			},
		},
		{
			// PodB confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(podBObj),
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

	Expect(testutil.EventsToExpEvents(destroyerEvents)).To(testutil.Equal(expEvents))

	By("verify podB deleted")
	assertUnstructuredDoesNotExist(ctx, c, podBObj)

	By("verify podA deleted")
	assertUnstructuredDoesNotExist(ctx, c, podAObj)
}

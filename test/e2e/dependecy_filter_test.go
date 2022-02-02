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
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//nolint:dupl // expEvents similar to other tests
func dependencyFilterTest(ctx context.Context, c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply resources in order based on depends-on annotation")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(inventoryName, namespaceName, "test"))

	pod1Obj := withDependsOn(withNamespace(manifestToUnstructured(pod1), namespaceName), fmt.Sprintf("/namespaces/%s/Pod/pod2", namespaceName))
	pod2Obj := withNamespace(manifestToUnstructured(pod2), namespaceName)

	// Dependency order: pod1 -> pod2
	// Apply order: pod2, pod1
	resources := []*unstructured.Unstructured{
		pod1Obj,
		pod2Obj,
	}

	// Cleanup
	defer func(ctx context.Context, c client.Client) {
		deleteUnstructuredIfExists(ctx, c, pod1Obj)
		deleteUnstructuredIfExists(ctx, c, pod2Obj)
	}(ctx, c)

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
			// Apply pod2 first
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
			// pod2 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// pod2 confirmed Current.
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
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-1",
				Type:      event.Started,
			},
		},
		{
			// Apply pod1 second
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-1",
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
			// pod1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// pod1 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
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

	By("verify pod1 created and ready")
	result := assertUnstructuredExists(ctx, c, pod1Obj)
	podIP, found, err := object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	By("verify pod2 created and ready")
	result = assertUnstructuredExists(ctx, c, pod2Obj)
	podIP, found, err = object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	// Attempt to Prune pod2
	resources = []*unstructured.Unstructured{
		pod1Obj,
	}

	applierEvents = runCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
		EmitStatusEvents: false,
	}))

	expEvents = []testutil.ExpEvent{
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
			// Apply pod1 first (Unchanged)
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Unchanged,
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
			// pod1 reconcile Skipped.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcileSkipped,
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
				Action:    event.PruneAction,
				GroupName: "prune-0",
				Type:      event.Started,
			},
		},
		{
			// Prune pod2 Skipped
			EventType: event.PruneType,
			PruneEvent: &testutil.ExpPruneEvent{
				GroupName:  "prune-0",
				Operation:  event.PruneSkipped,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.PruneAction,
				GroupName: "prune-0",
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
			// pod2 reconcile Skipped.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileSkipped,
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

	By("verify pod1 not deleted")
	result = assertUnstructuredExists(ctx, c, pod1Obj)
	ts, found, err := object.NestedField(result.Object, "metadata", "deletionTimestamp")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeFalse(), "deletionTimestamp found: ", ts)

	By("verify pod2 not deleted")
	result = assertUnstructuredExists(ctx, c, pod2Obj)
	ts, found, err = object.NestedField(result.Object, "metadata", "deletionTimestamp")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeFalse(), "deletionTimestamp found: ", ts)
}

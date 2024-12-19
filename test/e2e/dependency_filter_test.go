// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//nolint:dupl // expEvents similar to other tests
func dependencyFilterTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("apply resources in order based on depends-on annotation")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.FactoryFunc(inventoryName, namespaceName, "test"))

	pod1Obj := e2eutil.WithDependsOn(e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName), fmt.Sprintf("/namespaces/%s/Pod/pod2", namespaceName))
	pod2Obj := e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod2), namespaceName)

	// Dependency order: pod1 -> pod2
	// Apply order: pod2, pod1
	resources := []*unstructured.Unstructured{
		pod1Obj,
		pod2Obj,
	}

	// Cleanup
	defer func(ctx context.Context, c client.Client) {
		e2eutil.DeleteUnstructuredIfExists(ctx, c, pod1Obj)
		e2eutil.DeleteUnstructuredIfExists(ctx, c, pod2Obj)
	}(ctx, c)

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
			// Apply pod2 first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
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
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
			},
		},
		{
			// pod2 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSuccessful,
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
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// pod1 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileSuccessful,
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
	receivedEvents := testutil.EventsToExpEvents(applierEvents)

	expEvents, receivedEvents = e2eutil.FilterOptionalEvents(expEvents, receivedEvents)

	Expect(receivedEvents).To(testutil.Equal(expEvents))

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

	// Attempt to Prune pod2
	resources = []*unstructured.Unstructured{
		pod1Obj,
	}

	applierEvents = e2eutil.RunCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
		EmitStatusEvents: false,
		ValidationPolicy: validation.SkipInvalid,
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
			// Apply pod1 Skipped (dependency actuation strategy mismatch)
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplySkipped,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
				Error: testutil.EqualError(&filter.DependencyActuationMismatchError{
					Object:           object.UnstructuredToObjMetadata(pod1Obj),
					Strategy:         actuation.ActuationStrategyApply,
					Relationship:     filter.RelationshipDependency,
					Relation:         object.UnstructuredToObjMetadata(pod2Obj),
					RelationStrategy: actuation.ActuationStrategyDelete,
				}),
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
			// pod1 reconcile Skipped (because apply skipped)
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSkipped,
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
			// Prune pod2 Skipped (dependency actuation strategy mismatch)
			EventType: event.PruneType,
			PruneEvent: &testutil.ExpPruneEvent{
				GroupName:  "prune-0",
				Status:     event.PruneSkipped,
				Identifier: object.UnstructuredToObjMetadata(pod2Obj),
				Error: testutil.EqualError(&filter.DependencyActuationMismatchError{
					Object:           object.UnstructuredToObjMetadata(pod2Obj),
					Strategy:         actuation.ActuationStrategyDelete,
					Relationship:     filter.RelationshipDependent,
					Relation:         object.UnstructuredToObjMetadata(pod1Obj),
					RelationStrategy: actuation.ActuationStrategyApply,
				}),
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
			// pod2 reconcile Skipped (because prune skipped)
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileSkipped,
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
	receivedEvents = testutil.EventsToExpEvents(applierEvents)

	expEvents, receivedEvents = e2eutil.FilterOptionalEvents(expEvents, receivedEvents)

	Expect(receivedEvents).To(testutil.Equal(expEvents))

	By("verify pod1 not deleted")
	result = e2eutil.AssertUnstructuredExists(ctx, c, pod1Obj)
	ts, found, err := object.NestedField(result.Object, "metadata", "deletionTimestamp")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeFalse(), "deletionTimestamp found: ", ts)

	By("verify pod2 not deleted")
	result = e2eutil.AssertUnstructuredExists(ctx, c, pod2Obj)
	ts, found, err = object.NestedField(result.Object, "metadata", "deletionTimestamp")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeFalse(), "deletionTimestamp found: ", ts)
}

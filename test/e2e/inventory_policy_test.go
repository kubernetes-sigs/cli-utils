// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func inventoryPolicyMustMatchTest(c client.Client, invConfig InventoryConfig, namespaceName string) {
	By("Apply first set of resources")
	applier := invConfig.ApplierFactoryFunc()

	firstInvName := randomString("first-inv-")
	firstInv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(firstInvName, namespaceName, firstInvName))
	firstResources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
	}

	runWithNoErr(applier.Run(context.TODO(), firstInv, firstResources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Apply second set of resources")
	secondInvName := randomString("second-inv-")
	secondInv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(secondInvName, namespaceName, secondInvName))
	secondResources := []*unstructured.Unstructured{
		updateReplicas(deploymentManifest(namespaceName), 6),
	}

	ch := applier.Run(context.TODO(), secondInv, secondResources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		InventoryPolicy:  inventory.InventoryPolicyMustMatch,
	})

	var events []event.Event
	for e := range ch {
		events = append(events, e)
	}

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
			// ApplyTask error: resource managed by another inventory
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(deploymentManifest(namespaceName)),
				Error: testutil.EqualErrorType(
					inventory.NewInventoryOverlapError(errors.New("test")),
				),
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
	received := testutil.EventsToExpEvents(events)

	// handle optional async InProgress StatusEvents
	expected := testutil.ExpEvent{
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
	received, matches := testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	Expect(received).To(testutil.Equal(expEvents))

	By("Verify resource wasn't updated")
	var d appsv1.Deployment
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      deploymentManifest(namespaceName).GetName(),
	}, &d)
	Expect(err).NotTo(HaveOccurred())
	Expect(d.Spec.Replicas).To(Equal(func(i int32) *int32 { return &i }(4)))

	invConfig.InvCountVerifyFunc(c, namespaceName, 2)
}

func inventoryPolicyAdoptIfNoInventoryTest(c client.Client, invConfig InventoryConfig, namespaceName string) {
	By("Create unmanaged resource")
	err := c.Create(context.TODO(), deploymentManifest(namespaceName))
	Expect(err).NotTo(HaveOccurred())

	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()

	invName := randomString("test-inv-")
	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(invName, namespaceName, invName))
	resources := []*unstructured.Unstructured{
		updateReplicas(deploymentManifest(namespaceName), 6),
	}

	ch := applier.Run(context.TODO(), inv, resources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		InventoryPolicy:  inventory.AdoptIfNoInventory,
	})

	var events []event.Event
	for e := range ch {
		events = append(events, e)
	}

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
			// Apply deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Configured,
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

	// handle optional async InProgress StatusEvents
	received := testutil.EventsToExpEvents(events)
	expected := testutil.ExpEvent{
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
	received, matches := testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	Expect(received).To(testutil.Equal(expEvents))

	By("Verify resource was updated and added to inventory")
	var d appsv1.Deployment
	err = c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      deploymentManifest(namespaceName).GetName(),
	}, &d)
	Expect(err).NotTo(HaveOccurred())
	Expect(d.Spec.Replicas).To(Equal(func(i int32) *int32 { return &i }(6)))
	Expect(d.ObjectMeta.Annotations["config.k8s.io/owning-inventory"]).To(Equal(invName))

	invConfig.InvCountVerifyFunc(c, namespaceName, 1)
	invConfig.InvSizeVerifyFunc(c, invName, namespaceName, invName, 1)
}

func inventoryPolicyAdoptAllTest(c client.Client, invConfig InventoryConfig, namespaceName string) {
	By("Apply an initial set of resources")
	applier := invConfig.ApplierFactoryFunc()

	firstInvName := randomString("first-inv-")
	firstInv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(firstInvName, namespaceName, firstInvName))
	firstResources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
	}

	runWithNoErr(applier.Run(context.TODO(), firstInv, firstResources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	}))

	By("Apply resources")
	secondInvName := randomString("test-inv-")
	secondInv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(secondInvName, namespaceName, secondInvName))
	secondResources := []*unstructured.Unstructured{
		updateReplicas(deploymentManifest(namespaceName), 6),
	}

	ch := applier.Run(context.TODO(), secondInv, secondResources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		InventoryPolicy:  inventory.AdoptAll,
	})

	var events []event.Event
	for e := range ch {
		events = append(events, e)
	}

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
			// Apply deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Configured,
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

	// handle optional async InProgress StatusEvents
	received := testutil.EventsToExpEvents(events)
	expected := testutil.ExpEvent{
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
	received, matches := testutil.RemoveEqualEvents(received, expected)
	Expect(matches).To(BeNumerically(">=", 1), "unexpected number of %q status events", status.CurrentStatus)

	Expect(received).To(testutil.Equal(expEvents))

	By("Verify resource was updated and added to inventory")
	var d appsv1.Deployment
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      deploymentManifest(namespaceName).GetName(),
	}, &d)
	Expect(err).NotTo(HaveOccurred())
	Expect(d.Spec.Replicas).To(Equal(func(i int32) *int32 { return &i }(6)))
	Expect(d.ObjectMeta.Annotations["config.k8s.io/owning-inventory"]).To(Equal(secondInvName))

	invConfig.InvCountVerifyFunc(c, namespaceName, 2)
}

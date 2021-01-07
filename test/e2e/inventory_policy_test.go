// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
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
	err := verifyEvents([]expEvent{
		{
			eventType: event.ApplyType,
			applyEvent: &expApplyEvent{
				applyEventType: event.ApplyEventResourceUpdate,
				operation:      event.Unchanged,
				identifier:     object.UnstructuredToObjMeta(deploymentManifest(namespaceName)),
				error:          inventory.NewInventoryOverlapError(fmt.Errorf("test")),
			},
		},
		{
			eventType: event.ApplyType,
			applyEvent: &expApplyEvent{
				applyEventType: event.ApplyEventCompleted,
			},
		},
	}, events)
	Expect(err).ToNot(HaveOccurred())

	By("Verify resource wasn't updated")
	var d appsv1.Deployment
	err = c.Get(context.TODO(), types.NamespacedName{
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
	err = verifyEvents([]expEvent{
		{
			eventType: event.ApplyType,
			applyEvent: &expApplyEvent{
				applyEventType: event.ApplyEventResourceUpdate,
				operation:      event.Configured,
				identifier:     object.UnstructuredToObjMeta(deploymentManifest(namespaceName)),
				error:          nil,
			},
		},
		{
			eventType: event.ApplyType,
			applyEvent: &expApplyEvent{
				applyEventType: event.ApplyEventCompleted,
			},
		},
	}, events)
	Expect(err).ToNot(HaveOccurred())

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
	invConfig.InvSizeVerifyFunc(c, invName, namespaceName, 1)
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
	err := verifyEvents([]expEvent{
		{
			eventType: event.ApplyType,
			applyEvent: &expApplyEvent{
				applyEventType: event.ApplyEventResourceUpdate,
				operation:      event.Configured,
				identifier:     object.UnstructuredToObjMeta(deploymentManifest(namespaceName)),
				error:          nil,
			},
		},
		{
			eventType: event.ApplyType,
			applyEvent: &expApplyEvent{
				applyEventType: event.ApplyEventCompleted,
			},
		},
	}, events)
	Expect(err).ToNot(HaveOccurred())

	By("Verify resource was updated and added to inventory")
	var d appsv1.Deployment
	err = c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      deploymentManifest(namespaceName).GetName(),
	}, &d)
	Expect(err).NotTo(HaveOccurred())
	Expect(d.Spec.Replicas).To(Equal(func(i int32) *int32 { return &i }(6)))
	Expect(d.ObjectMeta.Annotations["config.k8s.io/owning-inventory"]).To(Equal(secondInvName))

	invConfig.InvCountVerifyFunc(c, namespaceName, 2)
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func applyAndDestroyTest(c client.Client, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := newApplier()

	inv := inventory.WrapInventoryInfoObj(cmInventoryManifest(inventoryName, namespaceName, "test"))

	resources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
	}

	applyCh := applier.Run(context.TODO(), inv, resources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
	})

	var applierEvents []event.Event
	for e := range applyCh {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
	err := verifyEvents([]expEvent{
		{
			eventType: event.InitType,
		},
		{
			eventType: event.ApplyType,
		},
		{
			eventType: event.ApplyType,
		},
		{
			eventType: event.PruneType,
		},
	}, applierEvents)
	Expect(err).ToNot(HaveOccurred())

	By("Verify inventory")
	var cm v1.ConfigMap
	err = c.Get(context.TODO(), types.NamespacedName{
		Name:      inventoryName,
		Namespace: namespaceName,
	}, &cm)
	Expect(err).ToNot(HaveOccurred())

	data := cm.Data
	Expect(len(data)).To(Equal(1))

	By("Destroy resources")
	destroyer := newDestroyer()
	err = destroyer.Initialize()
	Expect(err).NotTo(HaveOccurred())

	destroyCh := destroyer.Run(inv)

	var destroyerEvents []event.Event
	for e := range destroyCh {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		destroyerEvents = append(destroyerEvents, e)
	}
	err = verifyEvents([]expEvent{
		{
			eventType: event.DeleteType,
		},
		{
			eventType: event.DeleteType,
		},
	}, destroyerEvents)
	Expect(err).ToNot(HaveOccurred())
}

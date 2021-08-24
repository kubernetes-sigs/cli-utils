// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"strings"

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

func dependsOnTest(_ client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply resources in order based on depends-on annotation")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	// Dependency order: configmap1 -> configmap3 -> configmap2
	// Apply order: configmap2, configmap3, configmap1
	resources := []*unstructured.Unstructured{
		manifestToUnstructured(configmap1),
		manifestToUnstructured(configmap2),
		manifestToUnstructured(configmap3),
	}

	ch := applier.Run(context.TODO(), inv, resources, apply.Options{
		EmitStatusEvents: false,
	})

	var applierEvents []event.Event
	for e := range ch {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
	err := testutil.VerifyEvents([]testutil.ExpEvent{
		{
			// Configmap2 is applied first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(configmap2)),
				Operation:  event.Created,
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
		},
		{
			// Configmap3 is applied second
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(configmap3)),
				Operation:  event.Created,
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
		},
		{
			// Configmap1 is applied third
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(configmap1)),
				Operation:  event.Created,
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
		},
	}, applierEvents)
	Expect(err).ToNot(HaveOccurred())

	By("destroy resources in opposite order")
	destroyer := invConfig.DestroyerFactoryFunc()
	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollectNoErr(destroyer.Run(inv, options))
	err = testutil.VerifyEvents([]testutil.ExpEvent{
		{
			// Initial event
			EventType: event.InitType,
		},
		{
			// PruneTask start
			EventType: event.ActionGroupType,
		},
		{
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(configmap1)),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
		},
		{
			// PruneTask start
			EventType: event.ActionGroupType,
		},
		{
			// Delete CRD after custom resource
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(configmap3)),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
		},
	}, destroyerEvents)
	Expect(err).ToNot(HaveOccurred())
}

var configmap1 = []byte(strings.TrimSpace(`
kind: ConfigMap
apiVersion: v1
metadata:
  name: configmap1
  namespace: default
  annotations:
    config.kubernetes.io/depends-on: /namespaces/default/ConfigMap/configmap3
`))

var configmap2 = []byte(strings.TrimSpace(`
kind: ConfigMap
apiVersion: v1
metadata:
  name: configmap2
  namespace: default
`))

var configmap3 = []byte(strings.TrimSpace(`
kind: ConfigMap
apiVersion: v1
metadata:
  name: configmap3
  namespace: default
  annotations:
    config.kubernetes.io/depends-on: /namespaces/default/ConfigMap/configmap2
`))

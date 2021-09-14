// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"strings"
	"time"

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

func crdTest(_ client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply a set of resources that includes both a crd and a cr")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	resources := []*unstructured.Unstructured{
		manifestToUnstructured(cr),
		manifestToUnstructured(crd),
	}

	ch := applier.Run(context.TODO(), inv, resources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: false,
	})

	var applierEvents []event.Event
	for e := range ch {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
	err := testutil.VerifyEvents([]testutil.ExpEvent{
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
		},
		{
			// Apply CRD before custom resource
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(crd)),
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
			// Apply custom resource after CRD
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(cr)),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
		},
	}, applierEvents)
	Expect(err).ToNot(HaveOccurred())

	By("destroy the resources, including the crd")
	// TODO: test timeout/cancel behavior
	ctx := context.TODO()
	destroyer := invConfig.DestroyerFactoryFunc()
	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollectNoErr(destroyer.Run(ctx, inv, options))
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
			// Delete custom resource first
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(cr)),
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
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(crd)),
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

var crd = []byte(strings.TrimSpace(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: examples.cli-utils.example.io
spec:
  conversion:
    strategy: None
  group: cli-utils.example.io
  names:
    kind: Example
    listKind: ExampleList
    plural: examples
    singular: example
  scope: Cluster
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: Example for cli-utils e2e tests
        properties:
          apiVersion:
            type: string
          kind:
            type: string
          metadata:
            type: object
          spec:
            description: Example for cli-utils e2e tests
            properties:
              replicas:
                description: Number of replicas 
                type: integer
            required:
            - replicas
            type: object
        type: object
    served: true
    storage: true
    subresources: {}
`))

var cr = []byte(strings.TrimSpace(`
apiVersion: cli-utils.example.io/v1alpha1
kind: Example
metadata:
  name: example-cr
spec:
  replicas: 4
`))

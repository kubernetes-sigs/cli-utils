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

//nolint:dupl // expEvents similar to mutation tests
func crdTest(ctx context.Context, _ client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply a set of resources that includes both a crd and a cr")
	applier := invConfig.ApplierFactoryFunc()

	inv := inventory.InventoryInfoFromObject(inventoryFactoryFunc(invConfig, inventoryName, namespaceName, "test"))

	crdObj := manifestToUnstructured(crd)
	crObj := manifestToUnstructured(cr)

	resources := []*unstructured.Unstructured{
		crObj,
		crdObj,
	}

	applierEvents := runCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
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
			// Apply CRD before custom resource
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(crdObj),
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
			// CRD reconcile Pending .
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(crdObj),
			},
		},
		{
			// CRD confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(crdObj),
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
			// Apply CRD before custom resource
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-1",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(crObj),
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
			// CR reconcile Pending .
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(crObj),
			},
		},
		{
			// CR confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(crObj),
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

	By("destroy the resources, including the crd")
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
			// Delete custom resource first
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-0",
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetadata(crObj),
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
			// CR reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(crObj),
			},
		},
		{
			// CR confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(crObj),
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
			// Delete CRD after custom resource
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-1",
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetadata(crdObj),
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
			// CRD reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(crdObj),
			},
		},
		{
			// CRD confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(crdObj),
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

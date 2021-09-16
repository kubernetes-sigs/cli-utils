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

	// Dependency order: pod1 -> pod3 -> pod2
	// Apply order: pod2, pod3, pod1
	resources := []*unstructured.Unstructured{
		manifestToUnstructured(pod1),
		manifestToUnstructured(pod2),
		manifestToUnstructured(pod3),
	}

	ch := applier.Run(context.TODO(), inv, resources, apply.Options{
		EmitStatusEvents: false,
	})

	var applierEvents []event.Event
	for e := range ch {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
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
			// Apply Pod2 first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod2)),
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
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-1",
				Type:   event.Started,
			},
		},
		{
			// Apply pod3 second
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod3)),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-1",
				Type:   event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-1",
				Type:   event.Started,
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-1",
				Type:   event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-2",
				Type:   event.Started,
			},
		},
		{
			// Apply pod1 third
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod1)),
				Error:      nil,
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.ApplyAction,
				Name:   "apply-2",
				Type:   event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-2",
				Type:   event.Started,
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-2",
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
	Expect(testutil.EventsToExpEvents(applierEvents)).To(testutil.Equal(expEvents))

	By("destroy resources in opposite order")
	destroyer := invConfig.DestroyerFactoryFunc()
	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollectNoErr(destroyer.Run(inv, options))

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
				Action: event.DeleteAction,
				Name:   "prune-0",
				Type:   event.Started,
			},
		},
		{
			// Delete pod1 first
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod1)),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.DeleteAction,
				Name:   "prune-0",
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
			// PruneTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.DeleteAction,
				Name:   "prune-1",
				Type:   event.Started,
			},
		},
		{
			// Delete pod3 second
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod3)),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.DeleteAction,
				Name:   "prune-1",
				Type:   event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-1",
				Type:   event.Started,
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-1",
				Type:   event.Finished,
			},
		},
		{
			// PruneTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.DeleteAction,
				Name:   "prune-2",
				Type:   event.Started,
			},
		},
		{
			// Delete pod2 third
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod2)),
				Error:      nil,
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.DeleteAction,
				Name:   "prune-2",
				Type:   event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-2",
				Type:   event.Started,
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.WaitAction,
				Name:   "wait-2",
				Type:   event.Finished,
			},
		},
		{
			// DeleteInvTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "delete-inventory-0",
				Type:   event.Started,
			},
		},
		{
			// DeleteInvTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action: event.InventoryAction,
				Name:   "delete-inventory-0",
				Type:   event.Finished,
			},
		},
	}

	Expect(testutil.EventsToExpEvents(destroyerEvents)).To(testutil.Equal(expEvents))
}

var pod1 = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod1
  namespace: default
  annotations:
    config.kubernetes.io/depends-on: /namespaces/default/Pod/pod3
spec:
  containers:
  - name: kubernetes-pause
    image: k8s.gcr.io/pause:2.0
`))

var pod2 = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod2
  namespace: default
spec:
  containers:
  - name: kubernetes-pause
    image: k8s.gcr.io/pause:2.0
`))

var pod3 = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod3
  namespace: default
  annotations:
    config.kubernetes.io/depends-on: /namespaces/default/Pod/pod2
spec:
  containers:
  - name: kubernetes-pause
    image: k8s.gcr.io/pause:2.0
`))

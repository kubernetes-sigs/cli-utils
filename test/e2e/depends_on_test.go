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
	err := testutil.VerifyEvents([]testutil.ExpEvent{
		{
			// Pod2 is applied first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod2)),
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
			// Pod3 is applied second
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod3)),
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
			// Pod1 is applied third
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod1)),
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
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(pod3)),
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

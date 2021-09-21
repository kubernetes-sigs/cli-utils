// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// High priority use cases:
// - IAMPolicyMember .spec.member injected with apply-time-mutation using a name
//   that contains Project .status.number
//   https://github.com/GoogleCloudPlatform/k8s-config-connector/issues/340
// - Service .spec.loadBalancerIP injected with apply-time-mutation from
//   ComputeAddress .spec.address
//   https://github.com/GoogleCloudPlatform/k8s-config-connector/issues/334
//
// However, since both of these use Config Connector resources, which use CRDs
// that are copyright to Google, we can't use them as e2e tests here. Instead,
// we test a toy example with a pod-a depending on pod-b, injecting the ip and
// port from pod-b into an environment variable of pod-a.

func mutationTest(c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply resources in order with substitutions based on apply-time-mutation annotation")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	// Dependency order: podA -> podB
	// Apply order: podB, podA
	resources := []*unstructured.Unstructured{
		withNamespace(manifestToUnstructured(podA), namespaceName),
		withNamespace(manifestToUnstructured(podB), namespaceName),
	}

	for _, obj := range resources {
		obj.SetNamespace(namespaceName)
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
			// Apply PodB first
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(podB), namespaceName)),
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
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(podA), namespaceName)),
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

	By("verify resource was mutated")
	var podBObj v1.Pod
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      manifestToUnstructured(podB).GetName(),
	}, &podBObj)
	Expect(err).NotTo(HaveOccurred())
	Expect(podBObj.Status.PodIP).NotTo(BeEmpty())
	Expect(podBObj.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(80)))
	host := fmt.Sprintf("%s:%d", podBObj.Status.PodIP, podBObj.Spec.Containers[0].Ports[0].ContainerPort)

	var podAObj v1.Pod
	err = c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      manifestToUnstructured(podA).GetName(),
	}, &podAObj)
	Expect(err).NotTo(HaveOccurred())
	Expect(podAObj.Spec.Containers[0].Env[0].Value).To(Equal(host))

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
			// Delete podA first
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(podA), namespaceName)),
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
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(podB), namespaceName)),
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

	By("verify resources deleted")
	err = c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      manifestToUnstructured(podB).GetName(),
	}, &podBObj)
	Expect(err).To(HaveOccurred())
	Expect(apierrors.ReasonForError(err)).To(Equal(metav1.StatusReasonNotFound))

	err = c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      manifestToUnstructured(podA).GetName(),
	}, &podAObj)
	Expect(err).To(HaveOccurred())
	Expect(apierrors.ReasonForError(err)).To(Equal(metav1.StatusReasonNotFound))
}

func withNamespace(obj *unstructured.Unstructured, namespace string) *unstructured.Unstructured {
	obj.SetNamespace(namespace)
	return obj
}

var podA = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod-a
  namespace: test
  annotations:
    config.kubernetes.io/apply-time-mutation: |
      - sourceRef:
          kind: Pod
          name: pod-b
        sourcePath: $.status.podIP
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-ip}
      - sourceRef:
          kind: Pod
          name: pod-b
        sourcePath: $.spec.containers[?(@.name=="nginx")].ports[?(@.name=="tcp")].containerPort
        targetPath: $.spec.containers[?(@.name=="nginx")].env[?(@.name=="SERVICE_HOST")].value
        token: ${pob-b-port}
spec:
  containers:
  - name: nginx
    image: nginx:1.21
    ports:
    - name: tcp
      containerPort: 80
    env:
    - name: SERVICE_HOST
      value: "${pob-b-ip}:${pob-b-port}"
`))

var podB = []byte(strings.TrimSpace(`
kind: Pod
apiVersion: v1
metadata:
  name: pod-b
  namespace: test
spec:
  containers:
  - name: nginx
    image: nginx:1.21
    ports:
    - name: tcp
      containerPort: 80
`))

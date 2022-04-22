// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
)

func reconciliationFailed(ctx context.Context, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	podObj := e2eutil.WithNodeSelector(e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName), "foo", "bar")
	resources := []*unstructured.Unstructured{
		podObj,
	}

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: false,
	}))

	expEvents := expectedPodEvents(podObj, event.ReconcileFailed)
	received := testutil.EventsToExpEvents(applierEvents)

	Expect(received).To(testutil.Equal(expEvents))
}

func reconciliationTimeout(ctx context.Context, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	podObj := e2eutil.PodWithImage(e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName), "kubernetes-pause", "does-not-exist")
	resources := []*unstructured.Unstructured{
		podObj,
	}

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 30 * time.Second,
		EmitStatusEvents: false,
	}))

	expEvents := expectedPodEvents(podObj, event.ReconcileTimeout)
	received := testutil.EventsToExpEvents(applierEvents)

	Expect(received).To(testutil.Equal(expEvents))
}

func expectedPodEvents(pod *unstructured.Unstructured, waitStatus event.WaitEventStatus) []testutil.ExpEvent {
	return []testutil.ExpEvent{
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
			// Create deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod),
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
			// Deployment reconcile Pending .
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod),
			},
		},
		{
			// Deployment confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     waitStatus,
				Identifier: object.UnstructuredToObjMetadata(pod),
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
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	applyerror "sigs.k8s.io/cli-utils/pkg/apply/error"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func continueOnErrorTest(c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply an invalid CRD")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	resources := []*unstructured.Unstructured{
		manifestToUnstructured(invalidCrd),
		withNamespace(manifestToUnstructured(pod1), namespaceName),
	}

	applierEvents := runCollect(applier.Run(context.TODO(), inv, resources, apply.Options{}))

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
			// Apply invalidCrd fails
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(invalidCrd)),
				Error: testutil.EqualErrorType(
					applyerror.NewApplyRunError(errors.New("failed to apply")),
				),
			},
		},
		{
			// Create pod1
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetaOrDie(withNamespace(manifestToUnstructured(pod1), namespaceName)),
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
		// Note: No WaitTask when apply fails
		// TODO: why no wait after create tho?
		// {
		// 	// WaitTask start
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Started,
		// 	},
		// },
		// {
		// 	// WaitTask finished
		// 	EventType: event.ActionGroupType,
		// 	ActionGroupEvent: &testutil.ExpActionGroupEvent{
		// 		Action: event.WaitAction,
		// 		Name:   "wait-0",
		// 		Type:   event.Finished,
		// 	},
		// },
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

	By("Verify pod1 created")
	assertUnstructuredExists(c, withNamespace(manifestToUnstructured(pod1), namespaceName))

	By("Verify CRD not created")
	assertUnstructuredDoesNotExist(c, manifestToUnstructured(invalidCrd))
}

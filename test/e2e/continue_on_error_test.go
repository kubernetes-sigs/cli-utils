// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"errors"
	"strings"

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

func continueOnErrorTest(_ client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply an invalid CRD")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	resources := []*unstructured.Unstructured{
		manifestToUnstructured(invalidCrd),
	}

	ch := applier.Run(context.TODO(), inv, resources, apply.Options{})

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
			// Apply object which fails
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMetaOrDie(manifestToUnstructured(invalidCrd)),
				Error: testutil.EqualErrorType(
					applyerror.NewApplyRunError(errors.New("failed to apply")),
				),
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
		// Note: No WaitTask when apply fails
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
}

var invalidCrd = []byte(strings.TrimSpace(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: invalidexamples.cli-utils.example.io
spec:
  conversion:
    strategy: None
  group: cli-utils.example.io
  names:
    kind: InvalidExample
    listKind: InvalidExampleList
    plural: invalidexamples
    singular: invalidexample
  scope: Cluster
`))

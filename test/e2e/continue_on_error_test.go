// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
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

func continueOnErrorTest(_ client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply a set of resources that includes an invalid CRD and a deployment")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	resources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
		manifestToUnstructured(invalidCrd),
	}

	ch := applier.Run(context.TODO(), inv, resources, apply.Options{})

	var applierEvents []event.Event
	for e := range ch {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
		applierEvents = append(applierEvents, e)
	}
	err := testutil.VerifyEvents([]testutil.ExpEvent{
		{
			EventType: event.InitType,
		},
		{
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Identifier: object.UnstructuredToObjMeta(manifestToUnstructured(invalidCrd)),
				Error:      fmt.Errorf("failed to apply"),
			},
		},
		{
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				Operation: event.Created,
			},
		},
	}, applierEvents)
	Expect(err).ToNot(HaveOccurred())

	By("destroy the resources")
	destroyer := invConfig.DestroyerFactoryFunc()
	options := apply.DestroyerOptions{InventoryPolicy: inventory.AdoptIfNoInventory}
	destroyerEvents := runCollectNoErr(destroyer.Run(inv, options))
	err = testutil.VerifyEvents([]testutil.ExpEvent{
		{
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				Operation: event.Deleted,
				Error:     nil,
			},
		},
	}, destroyerEvents)
	Expect(err).ToNot(HaveOccurred())
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

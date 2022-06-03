// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func exitEarlyTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("exit early on invalid object")
	applier := invConfig.ApplierFactoryFunc()

	inv := inventory.InfoFromObject(invConfig.FactoryFunc(inventoryName, namespaceName, "test"))

	fields := struct{ Namespace string }{Namespace: namespaceName}
	// valid pod
	pod1Obj := e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName)
	// valid deployment with dependency
	deployment1Obj := e2eutil.WithDependsOn(e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName),
		fmt.Sprintf("/namespaces/%s/Pod/%s", namespaceName, pod1Obj.GetName()))
	// missing name
	invalidPodObj := e2eutil.TemplateToUnstructured(invalidPodTemplate, fields)

	resources := []*unstructured.Unstructured{
		pod1Obj,
		deployment1Obj,
		invalidPodObj,
	}

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
		EmitStatusEvents: false,
		ValidationPolicy: validation.ExitEarly,
	}))

	expEvents := []testutil.ExpEvent{
		{
			// invalid pod validation error
			EventType: event.ErrorType,
			ErrorEvent: &testutil.ExpErrorEvent{
				Err: testutil.EqualError(
					validation.NewError(
						field.Required(field.NewPath("metadata", "name"), "name is required"),
						object.UnstructuredToObjMetadata(invalidPodObj),
					),
				),
			},
		},
	}
	Expect(testutil.EventsToExpEvents(applierEvents)).To(testutil.Equal(expEvents))

	By("verify pod1 not found")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, pod1Obj)

	By("verify deployment1 not found")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, deployment1Obj)
}

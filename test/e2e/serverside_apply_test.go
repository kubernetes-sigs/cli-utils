// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func serversideApplyTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply a Deployment and an APIService by server-side apply")
	applier := invConfig.ApplierFactoryFunc()

	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)
	inventoryInfo, err := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)
	Expect(err).ToNot(HaveOccurred())

	firstResources := []*unstructured.Unstructured{
		e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName),
		e2eutil.ManifestToUnstructured(apiservice1),
	}

	e2eutil.RunWithNoErr(applier.Run(ctx, inventoryInfo, firstResources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		ServerSideOptions: common.ServerSideOptions{
			ServerSideApply: true,
			ForceConflicts:  true,
			FieldManager:    "test",
		},
	}))

	By("Verify deployment is server-side applied")
	result := e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName))

	// LastAppliedConfigAnnotation annotation is only set for client-side apply and we've server-side applied here.
	_, found, err := object.NestedField(result.Object, "metadata", "annotations", v1.LastAppliedConfigAnnotation)
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeFalse())

	manager, found, err := object.NestedField(result.Object, "metadata", "managedFields", 0, "manager")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(manager).To(Equal("test"))

	By("Verify APIService is server-side applied")
	result = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.ManifestToUnstructured(apiservice1))

	// LastAppliedConfigAnnotation annotation is only set for client-side apply and we've server-side applied here.
	_, found, err = object.NestedField(result.Object, "metadata", "annotations", v1.LastAppliedConfigAnnotation)
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeFalse())

	manager, found, err = object.NestedField(result.Object, "metadata", "managedFields", 0, "manager")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(manager).To(Equal("test"))
}

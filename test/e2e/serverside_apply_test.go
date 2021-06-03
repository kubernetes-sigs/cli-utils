// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func serversideApplyTest(c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("Apply a Deployment and an APIService by server-side apply")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))
	firstResources := []*unstructured.Unstructured{
		deploymentManifest(namespaceName),
		apiserviceManifest(),
	}

	runWithNoErr(applier.Run(context.TODO(), inv, firstResources, apply.Options{
		ReconcileTimeout: 2 * time.Minute,
		EmitStatusEvents: true,
		ServerSideOptions: common.ServerSideOptions{
			ServerSideApply: true,
			ForceConflicts:  true,
			FieldManager:    "test",
		},
	}))

	By("Verify deployment is server-side applied")
	var d appsv1.Deployment
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: namespaceName,
		Name:      deploymentManifest(namespaceName).GetName(),
	}, &d)
	Expect(err).NotTo(HaveOccurred())
	_, found := d.ObjectMeta.Annotations[v1.LastAppliedConfigAnnotation]
	Expect(found).To(BeFalse())
	fields := d.GetManagedFields()
	Expect(fields[0].Manager).To(Equal("test"))

	By("Verify APIService is server-side applied")
	var apiService = &unstructured.Unstructured{}
	apiService.SetGroupVersionKind(
		schema.GroupVersionKind{
			Group:   "apiregistration.k8s.io",
			Version: "v1",
			Kind:    "APIService",
		},
	)
	err = c.Get(context.TODO(), types.NamespacedName{
		Name: "v1beta1.custom.metrics.k8s.io",
	}, apiService)
	Expect(err).NotTo(HaveOccurred())
	_, found2 := apiService.GetAnnotations()[v1.LastAppliedConfigAnnotation]
	Expect(found2).To(BeFalse())
	fields2 := apiService.GetManagedFields()
	Expect(fields2[0].Manager).To(Equal("test"))
}

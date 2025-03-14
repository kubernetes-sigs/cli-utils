// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package invconfig

import (
	"context"
	"strings"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/test/e2e/customprovider"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCustomTypeInvConfig(cfg *rest.Config) InventoryConfig {
	return InventoryConfig{
		ClientConfig:         cfg,
		FactoryFunc:          customInventoryManifest,
		InvWrapperFunc:       customprovider.WrapInventoryInfoObj,
		ApplierFactoryFunc:   newCustomInvApplierFactory(cfg),
		DestroyerFactoryFunc: newCustomInvDestroyerFactory(cfg),
		InvSizeVerifyFunc:    customInvSizeVerifyFunc,
		InvCountVerifyFunc:   customInvCountVerifyFunc,
		InvNotExistsFunc:     customInvNotExistsFunc,
	}
}

func newCustomInvApplierFactory(cfg *rest.Config) applierFactoryFunc {
	cfgPtrCopy := cfg
	return func() *apply.Applier {
		return newApplier(customprovider.CustomClientFactory{}, inventory.StatusPolicyAll, cfgPtrCopy)
	}
}

func newCustomInvDestroyerFactory(cfg *rest.Config) destroyerFactoryFunc {
	cfgPtrCopy := cfg
	return func() *apply.Destroyer {
		return newDestroyer(customprovider.CustomClientFactory{}, inventory.StatusPolicyAll, cfgPtrCopy)
	}
}

func customInvNotExistsFunc(ctx context.Context, c client.Client, name, namespace, id string) {
	var u unstructured.Unstructured
	u.SetGroupVersionKind(customprovider.InventoryGVK)
	u.SetName(name)
	u.SetNamespace(namespace)
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, &u)
}

func customInvSizeVerifyFunc(ctx context.Context, c client.Client, name, namespace, _ string, specCount, statusCount int) {
	var u unstructured.Unstructured
	u.SetGroupVersionKind(customprovider.InventoryGVK)
	err := c.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &u)
	gomega.Expect(err).WithOffset(1).ToNot(gomega.HaveOccurred(), "getting custom inventory from cluster")

	s, found, err := unstructured.NestedSlice(u.Object, "spec", "objects")
	gomega.Expect(err).WithOffset(1).ToNot(gomega.HaveOccurred(), "reading inventory spec.objects")
	if found {
		gomega.Expect(len(s)).WithOffset(1).To(gomega.Equal(specCount), "inventory status.objects length")
	} else {
		gomega.Expect(specCount).WithOffset(1).To(gomega.Equal(0), "inventory spec.objects not found")
	}

	s, found, err = unstructured.NestedSlice(u.Object, "status", "objects")
	gomega.Expect(err).WithOffset(1).ToNot(gomega.HaveOccurred(), "reading inventory status.objects")
	if found {
		gomega.Expect(len(s)).WithOffset(1).To(gomega.Equal(statusCount), "inventory status.objects length")
	} else {
		gomega.Expect(statusCount).WithOffset(1).To(gomega.Equal(0), "inventory status.objects not found")
	}
}

func customInvCountVerifyFunc(ctx context.Context, c client.Client, namespace string, count int) {
	var u unstructured.UnstructuredList
	u.SetGroupVersionKind(customprovider.InventoryGVK)
	err := c.List(ctx, &u, client.InNamespace(namespace))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(len(u.Items)).To(gomega.Equal(count))
}

func cmInventoryManifest(name, namespace, id string) *unstructured.Unstructured {
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				common.InventoryLabel: id,
			},
		},
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{
		Object: u,
	}
}

func customInventoryManifest(name, namespace, id string) *unstructured.Unstructured {
	u := e2eutil.ManifestToUnstructured([]byte(strings.TrimSpace(`
apiVersion: cli-utils.example.io/v1alpha1
kind: Inventory
metadata:
  name: PLACEHOLDER
`)))
	u.SetName(name)
	u.SetNamespace(namespace)
	u.SetLabels(map[string]string{
		common.InventoryLabel: id,
	})
	return u
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/provider"
	"sigs.k8s.io/cli-utils/pkg/util/factory"
	"sigs.k8s.io/cli-utils/test/e2e/customprovider"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type inventoryFactoryFunc func(name, namespace, id string) *unstructured.Unstructured
type invWrapperFunc func(*unstructured.Unstructured) inventory.InventoryInfo
type applierFactoryFunc func() *apply.Applier
type destroyerFactoryFunc func() *apply.Destroyer
type invSizeVerifyFunc func(c client.Client, name, namespace, id string, count int)
type invCountVerifyFunc func(c client.Client, namespace string, count int)

type InventoryConfig struct {
	InventoryStrategy    inventory.InventoryStrategy
	InventoryFactoryFunc inventoryFactoryFunc
	InvWrapperFunc       invWrapperFunc
	ApplierFactoryFunc   applierFactoryFunc
	DestroyerFactoryFunc destroyerFactoryFunc
	InvSizeVerifyFunc    invSizeVerifyFunc
	InvCountVerifyFunc   invCountVerifyFunc
}

const (
	ConfigMapTypeInvConfig = "ConfigMap"
	CustomTypeInvConfig    = "Custom"
)

var inventoryConfigs = map[string]InventoryConfig{
	ConfigMapTypeInvConfig: {
		InventoryStrategy:    inventory.LabelStrategy,
		InventoryFactoryFunc: cmInventoryManifest,
		InvWrapperFunc:       inventory.WrapInventoryInfoObj,
		ApplierFactoryFunc:   newDefaultInvApplier,
		DestroyerFactoryFunc: newDefaultInvDestroyer,
		InvSizeVerifyFunc:    defaultInvSizeVerifyFunc,
		InvCountVerifyFunc:   defaultInvCountVerifyFunc,
	},
	CustomTypeInvConfig: {
		InventoryStrategy:    inventory.NameStrategy,
		InventoryFactoryFunc: customInventoryManifest,
		InvWrapperFunc:       customprovider.WrapInventoryInfoObj,
		ApplierFactoryFunc:   newCustomInvApplier,
		DestroyerFactoryFunc: newCustomInvDestroyer,
		InvSizeVerifyFunc:    customInvSizeVerifyFunc,
		InvCountVerifyFunc:   customInvCountVerifyFunc,
	},
}

var _ = Describe("Applier", func() {

	var c client.Client

	BeforeSuite(func() {
		cfg, err := ctrl.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		mapper, err := apiutil.NewDynamicRESTMapper(cfg)
		Expect(err).NotTo(HaveOccurred())

		c, err = client.New(cfg, client.Options{
			Scheme: scheme.Scheme,
			Mapper: mapper,
		})
		Expect(err).NotTo(HaveOccurred())

		createInventoryCRD(c)
	})

	for name := range inventoryConfigs {
		invConfig := inventoryConfigs[name]
		Context(fmt.Sprintf("Inventory: %s", name), func() {
			Context("Apply and destroy", func() {
				var namespace *v1.Namespace
				var inventoryName string

				BeforeEach(func() {
					inventoryName = randomString("test-inv-")
					namespace = createRandomNamespace(c)
				})

				AfterEach(func() {
					deleteNamespace(c, namespace)
				})

				It("Apply and destroy", func() {
					applyAndDestroyTest(c, invConfig, inventoryName, namespace.GetName())
				})

				It("Apply CRD and CR", func() {
					crdTest(c, invConfig, inventoryName, namespace.GetName())
				})

				It("Apply continues on error", func() {
					continueOnErrorTest(c, invConfig, inventoryName, namespace.GetName())
				})

				It("Server-Side Apply", func() {
					serversideApplyTest(c, invConfig, inventoryName, namespace.GetName())
				})
			})

			Context("Inventory policy", func() {
				var namespace *v1.Namespace

				BeforeEach(func() {
					namespace = createRandomNamespace(c)
				})

				AfterEach(func() {
					deleteNamespace(c, namespace)
				})

				It("MustMatch policy", func() {
					inventoryPolicyMustMatchTest(c, invConfig, namespace.GetName())
				})

				It("AdoptIfNoInventory policy", func() {
					inventoryPolicyAdoptIfNoInventoryTest(c, invConfig, namespace.GetName())
				})

				It("AdoptAll policy", func() {
					inventoryPolicyAdoptAllTest(c, invConfig, namespace.GetName())
				})
			})
		})
	}

	Context("InventoryStrategy: Name", func() {
		var namespace *v1.Namespace
		var inventoryName string

		BeforeEach(func() {
			inventoryName = randomString("test-inv-")
			namespace = createRandomNamespace(c)
		})

		AfterEach(func() {
			deleteNamespace(c, namespace)
		})

		It("Apply with existing inventory", func() {
			applyWithExistingInvTest(c, inventoryConfigs[CustomTypeInvConfig], inventoryName, namespace.GetName())
		})
	})
})

func createInventoryCRD(c client.Client) {
	invCRD := manifestToUnstructured(customprovider.InventoryCRD)
	var u unstructured.Unstructured
	u.SetGroupVersionKind(invCRD.GroupVersionKind())
	err := c.Get(context.TODO(), types.NamespacedName{
		Name: invCRD.GetName(),
	}, &u)
	if apierrors.IsNotFound(err) {
		err = c.Create(context.TODO(), invCRD)
	}
	Expect(err).NotTo(HaveOccurred())
}

func createRandomNamespace(c client.Client) *v1.Namespace {
	namespaceName := randomString("e2e-test-")
	namespace := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	err := c.Create(context.TODO(), namespace)
	Expect(err).ToNot(HaveOccurred())
	return namespace
}

func deleteNamespace(c client.Client, namespace *v1.Namespace) {
	err := c.Delete(context.TODO(), namespace)
	Expect(err).ToNot(HaveOccurred())
}

func newDefaultInvApplier() *apply.Applier {
	return newApplierFromProvider(newDefaultInvProvider())
}

func newDefaultInvDestroyer() *apply.Destroyer {
	destroyer := apply.NewDestroyer(newDefaultInvProvider())
	err := destroyer.Initialize()
	Expect(err).NotTo(HaveOccurred())
	return destroyer
}

func newDefaultInvProvider() provider.Provider {
	return provider.NewProvider(newFactory())
}

func defaultInvSizeVerifyFunc(c client.Client, name, namespace, id string, count int) {
	var cmList v1.ConfigMapList
	err := c.List(context.TODO(), &cmList,
		client.MatchingLabels(map[string]string{common.InventoryLabel: id}),
		client.InNamespace(namespace))
	Expect(err).ToNot(HaveOccurred())

	Expect(len(cmList.Items)).To(Equal(1))
	cm := cmList.Items[0]
	Expect(err).ToNot(HaveOccurred())

	data := cm.Data
	Expect(len(data)).To(Equal(count))
}

func defaultInvCountVerifyFunc(c client.Client, namespace string, count int) {
	var cmList v1.ConfigMapList
	err := c.List(context.TODO(), &cmList, client.InNamespace(namespace), client.HasLabels{common.InventoryLabel})
	Expect(err).NotTo(HaveOccurred())
	Expect(len(cmList.Items)).To(Equal(count))
}

func newCustomInvApplier() *apply.Applier {
	return newApplierFromProvider(newCustomInvProvider())
}

func newCustomInvDestroyer() *apply.Destroyer {
	destroyer := apply.NewDestroyer(newCustomInvProvider())
	err := destroyer.Initialize()
	Expect(err).NotTo(HaveOccurred())
	return destroyer
}

func newCustomInvProvider() provider.Provider {
	return customprovider.NewCustomProvider(newFactory())
}

func newFactory() util.Factory {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(&factory.CachingRESTClientGetter{
		Delegate: kubeConfigFlags,
	})
	return util.NewFactory(matchVersionKubeConfigFlags)
}

func customInvSizeVerifyFunc(c client.Client, name, namespace, _ string, count int) {
	var u unstructured.Unstructured
	u.SetGroupVersionKind(customprovider.InventoryGVK)
	err := c.Get(context.TODO(), types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &u)
	Expect(err).ToNot(HaveOccurred())

	s, found, err := unstructured.NestedSlice(u.Object, "spec", "inventory")
	Expect(err).ToNot(HaveOccurred())

	if !found {
		Expect(count).To(Equal(0))
		return
	}

	Expect(len(s)).To(Equal(count))
}

func customInvCountVerifyFunc(c client.Client, namespace string, count int) {
	var u unstructured.UnstructuredList
	u.SetGroupVersionKind(customprovider.InventoryGVK)
	err := c.List(context.TODO(), &u, client.InNamespace(namespace))
	Expect(err).NotTo(HaveOccurred())
	Expect(len(u.Items)).To(Equal(count))
}

func newApplierFromProvider(prov provider.Provider) *apply.Applier {
	statusPoller, err := factory.NewStatusPoller(prov.Factory())
	Expect(err).NotTo(HaveOccurred())

	a, err := apply.NewApplier(prov, statusPoller)
	Expect(err).NotTo(HaveOccurred())
	return a
}

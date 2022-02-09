// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/test/e2e/customprovider"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

type applierFactoryFunc func() *apply.Applier
type destroyerFactoryFunc func() *apply.Destroyer

type InventoryConfig struct {
	ApplierFactoryFunc   applierFactoryFunc
	DestroyerFactoryFunc destroyerFactoryFunc
	Converter            inventory.Converter
}

const (
	ConfigMapTypeInvConfig = "ConfigMap"
	CustomTypeInvConfig    = "Custom"
)

var inventoryConfigs = map[string]InventoryConfig{
	ConfigMapTypeInvConfig: {
		ApplierFactoryFunc:   newDefaultInvApplier,
		DestroyerFactoryFunc: newDefaultInvDestroyer,
		Converter:            inventory.ConfigMapConverter{},
	},
	CustomTypeInvConfig: {
		ApplierFactoryFunc:   newCustomInvApplier,
		DestroyerFactoryFunc: newCustomInvDestroyer,
		Converter:            customprovider.CustomConverter{},
	},
}

// Parse optional logging flags
// Ex: ginkgo ./test/e2e/... -- -v=5
// Allow init for e2e test (not imported by external code)
// nolint:gochecknoinits
func init() {
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)
}

var defaultTestTimeout = 5 * time.Minute
var defaultBeforeTestTimeout = 30 * time.Second
var defaultAfterTestTimeout = 30 * time.Second

var _ = Describe("Applier", func() {

	var c client.Client

	BeforeSuite(func() {
		// increase from 4000 to handle long event lists
		format.MaxLength = 10000

		cfg, err := ctrl.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		mapper, err := apiutil.NewDynamicRESTMapper(cfg)
		Expect(err).NotTo(HaveOccurred())

		c, err = client.New(cfg, client.Options{
			Scheme: scheme.Scheme,
			Mapper: mapper,
		})
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithTimeout(context.Background(), defaultBeforeTestTimeout)
		defer cancel()
		createInventoryCRD(ctx, c)
		Expect(ctx.Err()).To(BeNil(), "BeforeSuite context cancelled or timed out")
	})

	AfterSuite(func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultAfterTestTimeout)
		defer cancel()
		deleteInventoryCRD(ctx, c)
		Expect(ctx.Err()).To(BeNil(), "AfterSuite context cancelled or timed out")
	})

	for name := range inventoryConfigs {
		invConfig := inventoryConfigs[name]
		Context(fmt.Sprintf("Inventory: %s", name), func() {
			Context("Apply and destroy", func() {
				var namespace *v1.Namespace
				var inventoryName string
				var ctx context.Context
				var cancel context.CancelFunc

				BeforeEach(func() {
					ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
					inventoryName = randomString("test-inv-")
					namespace = createRandomNamespace(ctx, c)
				})

				AfterEach(func() {
					Expect(ctx.Err()).To(BeNil(), "test context cancelled or timed out")
					cancel()
					// new timeout for cleanup
					ctx, cancel = context.WithTimeout(context.Background(), defaultAfterTestTimeout)
					defer cancel()
					// clean up resources created by the tests
					fields := struct{ Namespace string }{Namespace: namespace.GetName()}
					objs := []*unstructured.Unstructured{
						manifestToUnstructured(cr),
						manifestToUnstructured(crd),
						withNamespace(manifestToUnstructured(pod1), namespace.GetName()),
						withNamespace(manifestToUnstructured(pod2), namespace.GetName()),
						withNamespace(manifestToUnstructured(pod3), namespace.GetName()),
						templateToUnstructured(podATemplate, fields),
						templateToUnstructured(podBTemplate, fields),
						withNamespace(manifestToUnstructured(deployment1), namespace.GetName()),
						manifestToUnstructured(apiservice1),
					}
					for _, obj := range objs {
						deleteUnstructuredIfExists(ctx, c, obj)
					}
					deleteNamespace(ctx, c, namespace)
				})

				It("Apply and destroy", func() {
					applyAndDestroyTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Deletion Prevention", func() {
					deletionPreventionTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Apply CRD and CR", func() {
					crdTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Apply continues on error", func() {
					continueOnErrorTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Server-Side Apply", func() {
					serversideApplyTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Implements depends-on apply ordering", func() {
					dependsOnTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Implements apply-time-mutation", func() {
					mutationTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Prune retrieval error correctly handled", func() {
					pruneRetrieveErrorTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("Reconciliation failure and timeout", func() {
					reconciliationFailed(ctx, invConfig, inventoryName, namespace.GetName())
				})

				It("Reconciliation timeout", func() {
					reconciliationTimeout(ctx, invConfig, inventoryName, namespace.GetName())
				})

				It("SkipInvalid", func() {
					skipInvalidTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("ExitEarly", func() {
					exitEarlyTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})
			})

			Context("Inventory policy", func() {
				var namespace *v1.Namespace
				var ctx context.Context
				var cancel context.CancelFunc

				BeforeEach(func() {
					ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
					namespace = createRandomNamespace(ctx, c)
				})

				AfterEach(func() {
					Expect(ctx.Err()).To(BeNil(), "test context cancelled or timed out")
					cancel()
					// new timeout for cleanup
					ctx, cancel = context.WithTimeout(context.Background(), defaultAfterTestTimeout)
					defer cancel()
					deleteUnstructuredIfExists(ctx, c, withNamespace(manifestToUnstructured(deployment1), namespace.GetName()))
					deleteNamespace(ctx, c, namespace)
				})

				It("MustMatch policy", func() {
					inventoryPolicyMustMatchTest(ctx, c, invConfig, namespace.GetName())
				})

				It("AdoptIfNoInventory policy", func() {
					inventoryPolicyAdoptIfNoInventoryTest(ctx, c, invConfig, namespace.GetName())
				})

				It("AdoptAll policy", func() {
					inventoryPolicyAdoptAllTest(ctx, c, invConfig, namespace.GetName())
				})
			})
		})
	}

	Context("InventoryStrategy: Name", func() {
		var namespace *v1.Namespace
		var inventoryName string
		var ctx context.Context
		var cancel context.CancelFunc

		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
			inventoryName = randomString("test-inv-")
			namespace = createRandomNamespace(ctx, c)
		})

		AfterEach(func() {
			Expect(ctx.Err()).To(BeNil(), "test context cancelled or timed out")
			cancel()
			// new timeout for cleanup
			ctx, cancel = context.WithTimeout(context.Background(), defaultAfterTestTimeout)
			defer cancel()
			deleteUnstructuredIfExists(ctx, c, withNamespace(manifestToUnstructured(deployment1), namespace.GetName()))
			deleteNamespace(ctx, c, namespace)
		})

		It("Apply with existing inventory", func() {
			applyWithExistingInvTest(ctx, c, inventoryConfigs[CustomTypeInvConfig], inventoryName, namespace.GetName())
		})
	})
})

func createInventoryCRD(ctx context.Context, c client.Client) {
	invCRD := manifestToUnstructured(customprovider.InventoryCRD)
	var u unstructured.Unstructured
	u.SetGroupVersionKind(invCRD.GroupVersionKind())
	err := c.Get(ctx, types.NamespacedName{
		Name: invCRD.GetName(),
	}, &u)
	if apierrors.IsNotFound(err) {
		err = c.Create(ctx, invCRD)
	}
	Expect(err).NotTo(HaveOccurred())
}

func createRandomNamespace(ctx context.Context, c client.Client) *v1.Namespace {
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

	err := c.Create(ctx, namespace)
	Expect(err).ToNot(HaveOccurred())
	return namespace
}

func deleteInventoryCRD(ctx context.Context, c client.Client) {
	invCRD := manifestToUnstructured(customprovider.InventoryCRD)
	deleteUnstructuredIfExists(ctx, c, invCRD)
}

func deleteUnstructuredIfExists(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	err := c.Delete(ctx, obj)
	if err != nil {
		Expect(err).To(Or(
			BeAssignableToTypeOf(&meta.NoKindMatchError{}),
			BeAssignableToTypeOf(&apierrors.StatusError{}),
		))
		if se, ok := err.(*apierrors.StatusError); ok {
			Expect(se.ErrStatus.Reason).To(Equal(metav1.StatusReasonNotFound))
		}
	}
}

func deleteNamespace(ctx context.Context, c client.Client, namespace *v1.Namespace) {
	err := c.Delete(ctx, namespace)
	Expect(err).ToNot(HaveOccurred())
}

func newDefaultInvApplier() *apply.Applier {
	return newApplierFromInvFactory(inventory.ConfigMapClientFactory{})
}

func newDefaultInvDestroyer() *apply.Destroyer {
	return newDestroyerFromInvFactory(inventory.ConfigMapClientFactory{})
}

func inventoryFactoryFunc(invConfig InventoryConfig, name, namespace, id string) *unstructured.Unstructured {
	inv := &actuation.Inventory{}
	inv.SetGroupVersionKind(invConfig.Converter.GroupVersionKind())
	inv.SetName(name)
	inv.SetNamespace(namespace)
	inv.SetLabels(map[string]string{
		common.InventoryLabel: id,
	})
	u, err := invConfig.Converter.From(inv)
	Expect(err).NotTo(HaveOccurred())
	return u
}

func invSizeVerifyFunc(ctx context.Context, c client.Client, invConfig InventoryConfig, name, namespace, id string, count int) {
	obj := inventoryFactoryFunc(invConfig, name, namespace, id)
	obj = assertUnstructuredExists(ctx, c, obj)
	inv, err := invConfig.Converter.To(obj)
	Expect(err).NotTo(HaveOccurred())
	Expect(inv.Spec.Objects).To(HaveLen(count))
}

func invCountVerifyFunc(ctx context.Context, c client.Client, invConfig InventoryConfig, namespace string, count int) {
	var u unstructured.UnstructuredList
	u.SetGroupVersionKind(invConfig.Converter.GroupVersionKind())
	err := c.List(ctx, &u, client.InNamespace(namespace), client.HasLabels{common.InventoryLabel})
	Expect(err).NotTo(HaveOccurred())
	Expect(len(u.Items)).To(Equal(count))
}

func newCustomInvApplier() *apply.Applier {
	return newApplierFromInvFactory(customprovider.CustomInventoryClientFactory{})
}

func newCustomInvDestroyer() *apply.Destroyer {
	return newDestroyerFromInvFactory(customprovider.CustomInventoryClientFactory{})
}

func newFactory() util.Factory {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(kubeConfigFlags)
	return util.NewFactory(matchVersionKubeConfigFlags)
}

func newApplierFromInvFactory(invFactory inventory.ClientFactory) *apply.Applier {
	f := newFactory()
	invClient, err := invFactory.NewClient(f)
	Expect(err).NotTo(HaveOccurred())

	a, err := apply.NewApplierBuilder().
		WithFactory(f).
		WithInventoryClient(invClient).
		Build()
	Expect(err).NotTo(HaveOccurred())
	return a
}

func newDestroyerFromInvFactory(invFactory inventory.ClientFactory) *apply.Destroyer {
	f := newFactory()
	invClient, err := invFactory.NewClient(f)
	Expect(err).NotTo(HaveOccurred())

	d, err := apply.NewDestroyer(f, invClient)
	Expect(err).NotTo(HaveOccurred())
	return d
}

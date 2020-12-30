// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/provider"
	"sigs.k8s.io/cli-utils/pkg/util/factory"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

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
	})

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
			applyAndDestroyTest(c, inventoryName, namespace.GetName())
		})

		It("Apply CRD and CR", func() {
			crdTest(c, inventoryName, namespace.GetName())
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
			inventoryPolicyMustMatchTest(c, namespace.GetName())
		})

		It("AdoptIfNoInventory policy", func() {
			inventoryPolicyAdoptIfNoInventoryTest(c, namespace.GetName())
		})

		It("AdoptAll policy", func() {
			inventoryPolicyAdoptAllTest(c, namespace.GetName())
		})
	})
})

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

func newApplier() *apply.Applier {
	applier := apply.NewApplier(newProvider())
	err := applier.Initialize()
	Expect(err).NotTo(HaveOccurred())
	return applier
}

func runWithNoErr(ch <-chan event.Event) {
	runCollectNoErr(ch)
}

func runCollect(ch <-chan event.Event) []event.Event {
	var events []event.Event
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func runCollectNoErr(ch <-chan event.Event) []event.Event {
	events := runCollect(ch)
	for _, e := range events {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
	}
	return events
}

func newDestroyer() *apply.Destroyer {
	destroyer := apply.NewDestroyer(newProvider())
	err := destroyer.Initialize()
	Expect(err).NotTo(HaveOccurred())
	return destroyer
}

func newProvider() provider.Provider {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(&factory.CachingRESTClientGetter{
		Delegate: kubeConfigFlags,
	})
	f := util.NewFactory(matchVersionKubeConfigFlags)
	return provider.NewProvider(f)
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
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

	var namespaceName string
	var namespace *v1.Namespace
	var inventoryName string

	BeforeEach(func() {
		namespaceName = randomString("e2e-test-")
		namespace = &v1.Namespace{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       "Namespace",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		inventoryName = randomString("test-inv-")
		err := c.Create(context.TODO(), namespace)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := c.Delete(context.TODO(), namespace)
		Expect(err).ToNot(HaveOccurred())
	})

	It("Apply and destroy", func() {
		By("Apply resources")
		applier := newApplier()
		err := applier.Initialize()
		Expect(err).NotTo(HaveOccurred())

		inv := inventory.WrapInventoryInfoObj(inventoryManifest(inventoryName, namespaceName))

		resources := []*unstructured.Unstructured{
			deploymentManifest(namespaceName),
		}

		applyCh := applier.Run(context.TODO(), inv, resources, apply.Options{
			ReconcileTimeout: 2 * time.Minute,
			EmitStatusEvents: true,
		})

		for e := range applyCh {
			Expect(e.Type).NotTo(Equal(event.ErrorType))
		}

		By("Verify inventory")
		var cm v1.ConfigMap
		err = c.Get(context.TODO(), types.NamespacedName{
			Name:      inventoryName,
			Namespace: namespaceName,
		}, &cm)
		Expect(err).ToNot(HaveOccurred())

		data := cm.Data
		Expect(len(data)).To(Equal(1))

		By("Destroy resources")
		destroyer := newDestroyer()
		err = destroyer.Initialize()
		Expect(err).NotTo(HaveOccurred())

		destroyCh := destroyer.Run(inv)

		for e := range destroyCh {
			Expect(e.Type).NotTo(Equal(event.ErrorType))
		}
	})
})

func inventoryManifest(name, namespace string) *unstructured.Unstructured {
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				common.InventoryLabel: "test",
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

func deploymentManifest(namespace string) *unstructured.Unstructured {
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-deployment",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func() *int32 { r := int32(4); return &r }(),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "nginx",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.19.6",
						},
					},
				},
			},
		},
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{
		Object: u,
	}
}

func newApplier() *apply.Applier {
	return apply.NewApplier(newProvider())
}

func newDestroyer() *apply.Destroyer {
	return apply.NewDestroyer(newProvider())
}

func newProvider() provider.Provider {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(&factory.CachingRESTClientGetter{
		Delegate: kubeConfigFlags,
	})
	f := util.NewFactory(matchVersionKubeConfigFlags)
	return provider.NewProvider(f)
}

func randomString(prefix string) string {
	seed := time.Now().UTC().UnixNano()
	randomSuffix := common.RandomStr(seed)
	return fmt.Sprintf("%s%s", prefix, randomSuffix)
}

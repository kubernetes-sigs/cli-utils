// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	"github.com/onsi/gomega/format"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const (
	ConfigMapTypeInvConfig = "ConfigMap"
	CustomTypeInvConfig    = "Custom"
)

var inventoryConfigs = map[string]invconfig.InventoryConfig{}
var inventoryConfigTypes = []string{
	ConfigMapTypeInvConfig,
	CustomTypeInvConfig,
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

var c client.Client

var _ = BeforeSuite(func() {
	// increase from 4000 to handle long event lists
	format.MaxLength = 10000

	cfg, err := ctrl.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	cfg.UserAgent = e2eutil.UserAgent("test/e2e")

	if e2eutil.IsFlowControlEnabled(cfg) {
		// Disable client-side throttling.
		klog.V(3).Infof("Client-side throttling disabled")
		cfg.QPS = -1
		cfg.Burst = -1
	}

	inventoryConfigs[ConfigMapTypeInvConfig] = invconfig.NewConfigMapTypeInvConfig(cfg)
	inventoryConfigs[CustomTypeInvConfig] = invconfig.NewCustomTypeInvConfig(cfg)

	httpClient, err := rest.HTTPClientFor(cfg)
	Expect(err).NotTo(HaveOccurred())
	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	Expect(err).NotTo(HaveOccurred())

	c, err = client.New(cfg, client.Options{
		Scheme: scheme.Scheme,
		Mapper: mapper,
	})
	Expect(err).NotTo(HaveOccurred())

	ctx, cancel := context.WithTimeout(context.Background(), defaultBeforeTestTimeout)
	defer cancel()
	e2eutil.CreateInventoryCRD(ctx, c)
	Expect(ctx.Err()).To(BeNil(), "BeforeSuite context cancelled or timed out")
})

var _ = AfterSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAfterTestTimeout)
	defer cancel()
	if c != nil {
		// If BeforeSuite() failed, c might be nil. Skip deletion to avoid red herring panic.
		e2eutil.DeleteInventoryCRD(ctx, c)
	}
	Expect(ctx.Err()).To(BeNil(), "AfterSuite context cancelled or timed out")
})

var _ = Describe("E2E", func() {
	for i := range inventoryConfigTypes {
		invType := inventoryConfigTypes[i]
		Context(fmt.Sprintf("Inventory%s", invType), func() {
			var invConfig invconfig.InventoryConfig

			BeforeEach(func() {
				invConfig = inventoryConfigs[invType]
			})

			Context("Basic", func() {
				var namespace *v1.Namespace
				var inventoryName string
				var ctx context.Context
				var cancel context.CancelFunc

				BeforeEach(func() {
					ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
					inventoryName = e2eutil.RandomString("test-inv-")
					namespace = e2eutil.CreateRandomNamespace(ctx, c)
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
						e2eutil.ManifestToUnstructured(cr),
						e2eutil.ManifestToUnstructured(crd),
						e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespace.GetName()),
						e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod2), namespace.GetName()),
						e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod3), namespace.GetName()),
						e2eutil.TemplateToUnstructured(podATemplate, fields),
						e2eutil.TemplateToUnstructured(podBTemplate, fields),
						e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespace.GetName()),
						e2eutil.ManifestToUnstructured(apiservice1),
					}
					for _, obj := range objs {
						e2eutil.DeleteUnstructuredIfExists(ctx, c, obj)
					}
					e2eutil.DeleteNamespace(ctx, c, namespace)
				})

				It("ApplyDestroy", func() {
					applyAndDestroyTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("DryRun", func() {
					dryRunTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("EmptySet", func() {
					emptySetTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("DeletionPrevention", func() {
					deletionPreventionTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("MutationPrevention", func() {
					mutationPreventionTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("CustomResource", func() {
					crdTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("ContinueOnError", func() {
					continueOnErrorTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("ServerSideApply", func() {
					serversideApplyTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("DependsOn", func() {
					dependsOnTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("ApplyTimeMutation", func() {
					mutationTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("DependencyFilter", func() {
					dependencyFilterTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("LocalNamespacesFilter", func() {
					namespaceFilterTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("CurrentUIDFilter", func() {
					currentUIDFilterTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("PruneRetrievalError", func() {
					pruneRetrieveErrorTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("DestroyReconciliationFailure", func() {
					destroyReconciliationFailureTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("ReconciliationFailure", func() {
					reconciliationFailed(ctx, invConfig, inventoryName, namespace.GetName())
				})

				It("ReconciliationTimeout", func() {
					reconciliationTimeout(ctx, invConfig, inventoryName, namespace.GetName())
				})

				It("SkipInvalid", func() {
					skipInvalidTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})

				It("ExitEarly", func() {
					exitEarlyTest(ctx, c, invConfig, inventoryName, namespace.GetName())
				})
			})

			Context("InventoryPolicy", func() {
				var namespace *v1.Namespace
				var ctx context.Context
				var cancel context.CancelFunc

				BeforeEach(func() {
					ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
					namespace = e2eutil.CreateRandomNamespace(ctx, c)
				})

				AfterEach(func() {
					Expect(ctx.Err()).To(BeNil(), "test context cancelled or timed out")
					cancel()
					// new timeout for cleanup
					ctx, cancel = context.WithTimeout(context.Background(), defaultAfterTestTimeout)
					defer cancel()
					e2eutil.DeleteUnstructuredIfExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespace.GetName()))
					e2eutil.DeleteNamespace(ctx, c, namespace)
				})

				It("MustMatch", func() {
					inventoryPolicyMustMatchTest(ctx, c, invConfig, namespace.GetName())
				})

				It("AdoptIfNoInventory", func() {
					inventoryPolicyAdoptIfNoInventoryTest(ctx, c, invConfig, namespace.GetName())
				})

				It("AdoptAll", func() {
					inventoryPolicyAdoptAllTest(ctx, c, invConfig, namespace.GetName())
				})
			})
		})
	}

	Context("NameStrategy", func() {
		var namespace *v1.Namespace
		var inventoryName string
		var ctx context.Context
		var cancel context.CancelFunc

		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), defaultTestTimeout)
			inventoryName = e2eutil.RandomString("test-inv-")
			namespace = e2eutil.CreateRandomNamespace(ctx, c)
		})

		AfterEach(func() {
			Expect(ctx.Err()).To(BeNil(), "test context cancelled or timed out")
			cancel()
			// new timeout for cleanup
			ctx, cancel = context.WithTimeout(context.Background(), defaultAfterTestTimeout)
			defer cancel()
			e2eutil.DeleteUnstructuredIfExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespace.GetName()))
			e2eutil.DeleteNamespace(ctx, c, namespace)
		})

		It("InventoryIDMismatch", func() {
			applyWithExistingInvTest(ctx, c, inventoryConfigs[CustomTypeInvConfig], inventoryName, namespace.GetName())
		})
	})
})

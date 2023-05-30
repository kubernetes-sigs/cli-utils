// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stress

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// Parse optional logging flags
// Ex: ginkgo ./test/e2e/... -- -v=5
// Allow init for e2e test (not imported by external code)
// nolint:gochecknoinits
func init() {
	klog.InitFlags(nil)
	klog.SetOutput(GinkgoWriter)
}

var defaultTestTimeout = 1 * time.Hour
var defaultBeforeTestTimeout = 30 * time.Second
var defaultAfterTestTimeout = 30 * time.Second

var c client.Client
var invConfig invconfig.InventoryConfig

var _ = BeforeSuite(func() {
	// increase from 4000 to handle long event lists
	format.MaxLength = 10000

	cfg, err := ctrl.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	cfg.UserAgent = e2eutil.UserAgent("test/stress")

	if e2eutil.IsFlowControlEnabled(cfg) {
		// Disable client-side throttling.
		klog.V(3).Infof("Client-side throttling disabled")
		cfg.QPS = -1
		cfg.Burst = -1
	}

	invConfig = invconfig.NewCustomTypeInvConfig(cfg)

	mapper, err := apiutil.NewDynamicRESTMapper(cfg, http.DefaultClient)
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

var _ = Describe("Stress", func() {
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
		ctx, cancel = context.WithTimeout(context.Background(), defaultAfterTestTimeout)
		defer cancel()
		// clean up resources created by the tests
		e2eutil.DeleteNamespace(ctx, c, namespace)
	})

	It("ThousandDeployments", func() {
		thousandDeploymentsTest(ctx, c, invConfig, inventoryName, namespace.GetName())
	})

	It("ThousandDeploymentsRetry", func() {
		thousandDeploymentsRetryTest(ctx, c, invConfig, inventoryName, namespace.GetName())
	})

	It("ThousandNamespaces", func() {
		thousandNamespacesTest(ctx, c, invConfig, inventoryName, namespace.GetName())
	})
})

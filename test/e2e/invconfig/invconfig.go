// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package invconfig

import (
	"context"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type inventoryFactoryFunc func(name, namespace, id string) *unstructured.Unstructured
type invWrapperFunc func(*unstructured.Unstructured) (inventory.Info, error)
type applierFactoryFunc func() *apply.Applier
type destroyerFactoryFunc func() *apply.Destroyer
type invSizeVerifyFunc func(ctx context.Context, c client.Client, name, namespace, id string, specCount, statusCount int)
type invCountVerifyFunc func(ctx context.Context, c client.Client, namespace string, count int)
type invNotExistsFunc func(ctx context.Context, c client.Client, name, namespace, id string)

type InventoryConfig struct {
	ClientConfig         *rest.Config
	FactoryFunc          inventoryFactoryFunc
	InvWrapperFunc       invWrapperFunc
	ApplierFactoryFunc   applierFactoryFunc
	DestroyerFactoryFunc destroyerFactoryFunc
	InvSizeVerifyFunc    invSizeVerifyFunc
	InvCountVerifyFunc   invCountVerifyFunc
	InvNotExistsFunc     invNotExistsFunc
}

func CreateInventoryInfo(invConfig InventoryConfig, inventoryName, namespaceName, inventoryID string) (inventory.Info, error) {
	return invConfig.InvWrapperFunc(invConfig.FactoryFunc(inventoryName, namespaceName, inventoryID))
}

func newFactory(cfg *rest.Config) util.Factory {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	cfgPtrCopy := cfg
	kubeConfigFlags.WrapConfigFn = func(c *rest.Config) *rest.Config {
		// update rest.Config to pick up QPS & timeout changes
		deepCopyRESTConfig(cfgPtrCopy, c)
		return c
	}
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(kubeConfigFlags)
	return util.NewFactory(matchVersionKubeConfigFlags)
}

func newApplier(invFactory inventory.ClientFactory, cfg *rest.Config) *apply.Applier {
	f := newFactory(cfg)
	invClient, err := invFactory.NewClient(f)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	a, err := apply.NewApplierBuilder().
		WithFactory(f).
		WithInventoryClient(invClient).
		Build()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return a
}

func newDestroyer(invFactory inventory.ClientFactory, cfg *rest.Config) *apply.Destroyer {
	f := newFactory(cfg)
	invClient, err := invFactory.NewClient(f)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	d, err := apply.NewDestroyerBuilder().
		WithFactory(f).
		WithInventoryClient(invClient).
		Build()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return d
}

func deepCopyRESTConfig(from, to *rest.Config) {
	to.Host = from.Host
	to.APIPath = from.APIPath
	to.ContentConfig = from.ContentConfig
	to.Username = from.Username
	to.Password = from.Password
	to.BearerToken = from.BearerToken
	to.BearerTokenFile = from.BearerTokenFile
	to.Impersonate = rest.ImpersonationConfig{
		UserName: from.Impersonate.UserName,
		UID:      from.Impersonate.UID,
		Groups:   from.Impersonate.Groups,
		Extra:    from.Impersonate.Extra,
	}
	to.AuthProvider = from.AuthProvider
	to.AuthConfigPersister = from.AuthConfigPersister
	to.ExecProvider = from.ExecProvider
	if from.ExecProvider != nil && from.ExecProvider.Config != nil {
		to.ExecProvider.Config = from.ExecProvider.Config.DeepCopyObject()
	}
	to.TLSClientConfig = rest.TLSClientConfig{
		Insecure:   from.TLSClientConfig.Insecure,
		ServerName: from.TLSClientConfig.ServerName,
		CertFile:   from.TLSClientConfig.CertFile,
		KeyFile:    from.TLSClientConfig.KeyFile,
		CAFile:     from.TLSClientConfig.CAFile,
		CertData:   from.TLSClientConfig.CertData,
		KeyData:    from.TLSClientConfig.KeyData,
		CAData:     from.TLSClientConfig.CAData,
		NextProtos: from.TLSClientConfig.NextProtos,
	}
	to.UserAgent = from.UserAgent
	to.DisableCompression = from.DisableCompression
	to.Transport = from.Transport
	to.WrapTransport = from.WrapTransport
	to.QPS = from.QPS
	to.Burst = from.Burst
	to.RateLimiter = from.RateLimiter
	to.WarningHandler = from.WarningHandler
	to.Timeout = from.Timeout
	to.Dial = from.Dial
	to.Proxy = from.Proxy
}

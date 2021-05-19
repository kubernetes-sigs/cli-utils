// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// CachingRESTClientGetter caches the RESTMapper so every call to
// ToRESTMapper will get a reference to the same mapper.
type CachingRESTClientGetter struct {
	mx       sync.Mutex
	Delegate genericclioptions.RESTClientGetter

	mapper meta.RESTMapper
}

func (c *CachingRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return c.Delegate.ToRESTConfig()
}

func (c *CachingRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return c.Delegate.ToDiscoveryClient()
}

func (c *CachingRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	if c.mapper != nil {
		return c.mapper, nil
	}
	var err error
	c.mapper, err = c.Delegate.ToRESTMapper()
	return c.mapper, err
}

func (c *CachingRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return c.Delegate.ToRawKubeConfigLoader()
}

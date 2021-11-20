// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type applierConfig struct {
	// factory is only used to retrieve things that have not been provided explicitly.
	factory                      util.Factory
	invClient                    inventory.InventoryClient
	client                       dynamic.Interface
	discoClient                  discovery.CachedDiscoveryInterface
	mapper                       meta.RESTMapper
	restConfig                   *rest.Config
	unstructuredClientForMapping func(*meta.RESTMapping) (resource.RESTClient, error)
	statusPoller                 poller.Poller
}

type ApplierOption func(*applierConfig)

func constructApplierConfig(opts []ApplierOption) (*applierConfig, error) {
	cfg := defaultApplierConfig()
	setOptsOnApplierConfig(cfg, opts)
	err := finalizeApplierConfig(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultApplierConfig() *applierConfig {
	return &applierConfig{}
}

func setOptsOnApplierConfig(cfg *applierConfig, opts []ApplierOption) {
	for _, opt := range opts {
		opt(cfg)
	}
}

func finalizeApplierConfig(cfg *applierConfig) error {
	var err error
	if cfg.invClient == nil {
		return errors.New("inventory client must be provided")
	}
	if cfg.client == nil {
		if cfg.factory == nil {
			return fmt.Errorf("a factory must be provided or all other options: %v", err)
		}
		cfg.client, err = cfg.factory.DynamicClient()
		if err != nil {
			return fmt.Errorf("error getting dynamic client: %v", err)
		}
	}
	if cfg.discoClient == nil {
		if cfg.factory == nil {
			return fmt.Errorf("a factory must be provided or all other options: %v", err)
		}
		cfg.discoClient, err = cfg.factory.ToDiscoveryClient()
		if err != nil {
			return fmt.Errorf("error getting discovery client: %v", err)
		}
	}
	if cfg.mapper == nil {
		if cfg.factory == nil {
			return fmt.Errorf("a factory must be provided or all other options: %v", err)
		}
		cfg.mapper, err = cfg.factory.ToRESTMapper()
		if err != nil {
			return fmt.Errorf("error getting rest mapper: %v", err)
		}
	}
	if cfg.restConfig == nil {
		if cfg.factory == nil {
			return fmt.Errorf("a factory must be provided or all other options: %v", err)
		}
		cfg.restConfig, err = cfg.factory.ToRESTConfig()
		if err != nil {
			return fmt.Errorf("error getting rest config: %v", err)
		}
	}
	if cfg.unstructuredClientForMapping == nil {
		if cfg.factory == nil {
			return fmt.Errorf("a factory must be provided or all other options: %v", err)
		}
		cfg.unstructuredClientForMapping = cfg.factory.UnstructuredClientForMapping
	}
	if cfg.statusPoller == nil {
		c, err := client.New(cfg.restConfig, client.Options{Scheme: scheme.Scheme, Mapper: cfg.mapper})
		if err != nil {
			return fmt.Errorf("error creating client: %v", err)
		}
		cfg.statusPoller = polling.NewStatusPoller(c, cfg.mapper)
	}
	return nil
}

func WithFactory(factory util.Factory) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.factory = factory
	}
}

func WithInventoryClient(invClient inventory.InventoryClient) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.invClient = invClient
	}
}

func WithDynamicClient(client dynamic.Interface) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.client = client
	}
}

func WithDiscoveryClient(discoClient discovery.CachedDiscoveryInterface) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.discoClient = discoClient
	}
}

func WithRestMapper(mapper meta.RESTMapper) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.mapper = mapper
	}
}

func WithRestConfig(restConfig *rest.Config) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.restConfig = restConfig
	}
}

func WithUnstructuredClientForMapping(unstructuredClientForMapping func(*meta.RESTMapping) (resource.RESTClient, error)) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.unstructuredClientForMapping = unstructuredClientForMapping
	}
}

func WithStatusPoller(statusPoller poller.Poller) ApplierOption {
	return func(cfg *applierConfig) {
		cfg.statusPoller = statusPoller
	}
}

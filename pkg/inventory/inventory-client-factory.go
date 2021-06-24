// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import cmdutil "k8s.io/kubectl/pkg/cmd/util"

var (
	_ InventoryClientFactory = ClusterInventoryClientFactory{}
)

// InventoryClientFactory is a factory that constructs new InventoryClient instances.
type InventoryClientFactory interface {
	NewInventoryClient(factory cmdutil.Factory) (InventoryClient, error)
}

// ClusterInventoryClientFactory is a factory that creates instances of ClusterInventoryClient inventory client.
type ClusterInventoryClientFactory struct {
}

func (ClusterInventoryClientFactory) NewInventoryClient(factory cmdutil.Factory) (InventoryClient, error) {
	return NewInventoryClient(factory, WrapInventoryObj, InvInfoToConfigMap)
}

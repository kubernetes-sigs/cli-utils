// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package customprovider

import (
	"fmt"

	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

var _ inventory.ClientFactory = CustomInventoryClientFactory{}

type CustomInventoryClientFactory struct {
}

func (CustomInventoryClientFactory) NewClient(factory util.Factory) (inventory.Client, error) {
	client, err := factory.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("error getting dynamic client: %v", err)
	}

	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("error getting rest mapper: %v", err)
	}
	return &inventory.ClusterClient{
		DynamicClient: client,
		Mapper:        mapper,
		Converter:     CustomConverter{},
	}, nil
}

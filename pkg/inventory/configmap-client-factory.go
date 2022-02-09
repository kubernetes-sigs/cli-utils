// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type ConfigMapClientFactory struct{}

var _ ClientFactory = ConfigMapClientFactory{}

func (cmcf ConfigMapClientFactory) NewClient(factory cmdutil.Factory) (Client, error) {
	client, err := factory.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("error getting dynamic client: %v", err)
	}

	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("error getting rest mapper: %v", err)
	}

	return &ClusterClient{
		DynamicClient: client,
		Mapper:        mapper,
		Converter:     ConfigMapConverter{},
	}, nil
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"fmt"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewStatusPoller creates a new StatusPoller instance from the
// passed in factory.
func NewStatusPoller(f cmdutil.Factory) (*polling.StatusPoller, error) {
	config, err := f.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("error getting RESTConfig: %w", err)
	}

	mapper, err := f.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("error getting RESTMapper: %w", err)
	}

	c, err := client.New(config, client.Options{Scheme: scheme.Scheme, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("error creating client: %w", err)
	}

	return polling.NewStatusPoller(c, mapper), nil
}

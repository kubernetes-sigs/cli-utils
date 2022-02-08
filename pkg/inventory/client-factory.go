// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import cmdutil "k8s.io/kubectl/pkg/cmd/util"

// ClientFactory is a factory that constructs new Client instances.
type ClientFactory interface {
	NewClient(factory cmdutil.Factory) (Client, error)
}

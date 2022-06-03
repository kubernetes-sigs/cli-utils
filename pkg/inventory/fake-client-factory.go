// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type FakeClientFactory object.ObjMetadataSet

var _ ClientFactory = FakeClientFactory{}

func (f FakeClientFactory) NewClient(cmdutil.Factory) (Client, error) {
	return NewFakeClient(object.ObjMetadataSet(f)), nil
}

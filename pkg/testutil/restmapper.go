// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewFakeRESTMapper(gvks ...schema.GroupVersionKind) meta.RESTMapper {
	var groupVersions []schema.GroupVersion
	for _, gvk := range gvks {
		groupVersions = append(groupVersions, gvk.GroupVersion())
	}
	mapper := meta.NewDefaultRESTMapper(groupVersions)
	for _, gvk := range gvks {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	return mapper
}

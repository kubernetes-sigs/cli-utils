// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"sort"

	"k8s.io/cli-runtime/pkg/resource"
)

type ResourceInfos []*resource.Info

var _ sort.Interface = ResourceInfos{}

func (a ResourceInfos) Len() int      { return len(a) }
func (a ResourceInfos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ResourceInfos) Less(i, j int) bool {
	if !getGvk(a[i]).Equals(getGvk(a[j])) {
		return getGvk(a[i]).IsLessThan(getGvk(a[j]))
	}
	return a[i].String() < a[j].String()
}

func getGvk(info *resource.Info) Gvk {
	gvk := info.Object.GetObjectKind().GroupVersionKind()
	return Gvk{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
}

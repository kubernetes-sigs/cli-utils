// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"sort"

	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
)

type AlphanumericObjectReferences []actuation.ObjectReference

var _ sort.Interface = AlphanumericObjectReferences{}

func (a AlphanumericObjectReferences) Len() int      { return len(a) }
func (a AlphanumericObjectReferences) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a AlphanumericObjectReferences) Less(i, j int) bool {
	if a[i].Group != a[j].Group {
		return a[i].Group < a[j].Group
	}
	if a[i].Kind != a[j].Kind {
		return a[i].Kind < a[j].Kind
	}
	if a[i].Namespace != a[j].Namespace {
		return a[i].Namespace < a[j].Namespace
	}
	return a[i].Name < a[j].Name
}

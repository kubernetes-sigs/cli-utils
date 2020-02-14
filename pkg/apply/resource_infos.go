// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"sort"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
)

type ResourceInfos []*resource.Info

var _ sort.Interface = ResourceInfos{}

func (a ResourceInfos) Len() int      { return len(a) }
func (a ResourceInfos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ResourceInfos) Less(i, j int) bool {
	x := a[i].Object.GetObjectKind().GroupVersionKind()
	o := a[j].Object.GetObjectKind().GroupVersionKind()
	if !Equals(x, o) {
		return IsLessThan(x, o)
	}
	return a[i].String() < a[j].String()
}

// An attempt to order things to help k8s, e.g.
// a Service should come before things that refer to it.
// Namespace should be first.
// In some cases order just specified to provide determinism.
var orderFirst = []string{
	"Namespace",
	"ResourceQuota",
	"StorageClass",
	"CustomResourceDefinition",
	"MutatingWebhookConfiguration",
	"ServiceAccount",
	"PodSecurityPolicy",
	"Role",
	"ClusterRole",
	"RoleBinding",
	"ClusterRoleBinding",
	"ConfigMap",
	"Secret",
	"Service",
	"LimitRange",
	"PriorityClass",
	"Deployment",
	"StatefulSet",
	"CronJob",
	"PodDisruptionBudget",
}

var orderLast = []string{
	"ValidatingWebhookConfiguration",
}

// getIndexByKind returns the index of the kind respecting the order
func getIndexByKind(kind string) int {
	m := map[string]int{}
	for i, n := range orderFirst {
		m[n] = -len(orderFirst) + i
	}
	for i, n := range orderLast {
		m[n] = 1 + i
	}
	return m[kind]
}

// Equals returns true if the GVK's have equal fields.
func Equals(x schema.GroupVersionKind, o schema.GroupVersionKind) bool {
	return x.Group == o.Group && x.Version == o.Version && x.Kind == o.Kind
}

// IsLessThan compares two GVK's as per orderFirst and orderLast, returns boolean result.
func IsLessThan(x schema.GroupVersionKind, o schema.GroupVersionKind) bool {
	indexI := getIndexByKind(x.Kind)
	indexJ := getIndexByKind(o.Kind)
	if indexI != indexJ {
		return indexI < indexJ
	}
	return x.String() < o.String()
}

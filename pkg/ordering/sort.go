// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package ordering

import (
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type SortableInfos []*resource.Info

var _ sort.Interface = SortableInfos{}

func (a SortableInfos) Len() int      { return len(a) }
func (a SortableInfos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a SortableInfos) Less(i, j int) bool {
	first, err := object.InfoToObjMeta(a[i])
	if err != nil {
		return false
	}
	second, err := object.InfoToObjMeta(a[j])
	if err != nil {
		return false
	}
	return less(first, second)
}

type SortableUnstructureds []*unstructured.Unstructured

var _ sort.Interface = SortableUnstructureds{}

func (a SortableUnstructureds) Len() int      { return len(a) }
func (a SortableUnstructureds) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a SortableUnstructureds) Less(i, j int) bool {
	first := object.UnstructuredToObjMeta(a[i])
	second := object.UnstructuredToObjMeta(a[j])
	return less(first, second)
}

type SortableMetas []object.ObjMetadata

var _ sort.Interface = SortableMetas{}

func (a SortableMetas) Len() int      { return len(a) }
func (a SortableMetas) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a SortableMetas) Less(i, j int) bool {
	return less(a[i], a[j])
}

func less(i, j object.ObjMetadata) bool {
	if !Equals(i.GroupKind, j.GroupKind) {
		return IsLessThan(i.GroupKind, j.GroupKind)
	}
	// In case of tie, compare the namespace and name combination so that the output
	// order is consistent irrespective of input order
	return i.Namespace+i.Name < j.Namespace+j.Name
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

func Equals(x schema.GroupKind, o schema.GroupKind) bool {
	return x.Group == o.Group && x.Kind == o.Kind
}

func IsLessThan(x schema.GroupKind, o schema.GroupKind) bool {
	indexI := getIndexByKind(x.Kind)
	indexJ := getIndexByKind(o.Kind)
	if indexI != indexJ {
		return indexI < indexJ
	}
	return x.String() < o.String()
}

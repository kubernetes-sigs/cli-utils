// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
)

func InfoToUnstructured(info *resource.Info) *unstructured.Unstructured {
	return info.Object.(*unstructured.Unstructured)
}

func UnstructuredToInfo(obj *unstructured.Unstructured) *resource.Info {
	return &resource.Info{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Source:    "unstructured",
		Object:    obj,
	}
}

func InfosToUnstructureds(infos []*resource.Info) []*unstructured.Unstructured {
	var objs []*unstructured.Unstructured
	for _, info := range infos {
		objs = append(objs, InfoToUnstructured(info))
	}
	return objs
}

func UnstructuredsToInfos(objs []*unstructured.Unstructured) []*resource.Info {
	var infos []*resource.Info
	for _, obj := range objs {
		infos = append(infos, UnstructuredToInfo(obj))
	}
	return infos
}

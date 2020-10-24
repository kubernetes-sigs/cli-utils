// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
)

func InfoToUnstructured(info *resource.Info) *unstructured.Unstructured {
	return info.Object.(*unstructured.Unstructured)
}

func UnstructuredToInfo(obj *unstructured.Unstructured) (*resource.Info, error) {
	accessor, _ := meta.Accessor(obj)
	annos := accessor.GetAnnotations()

	source := "unstructured"
	path, ok := annos[kioutil.PathAnnotation]
	if ok {
		source = path
		delete(annos, kioutil.PathAnnotation)
		accessor.SetAnnotations(annos)
	}

	return &resource.Info{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Source:    source,
		Object:    obj,
	}, nil
}

func InfosToUnstructureds(infos []*resource.Info) []*unstructured.Unstructured {
	var objs []*unstructured.Unstructured
	for _, info := range infos {
		objs = append(objs, InfoToUnstructured(info))
	}
	return objs
}

func UnstructuredsToInfos(objs []*unstructured.Unstructured) ([]*resource.Info, error) {
	var infos []*resource.Info
	for _, obj := range objs {
		inf, err := UnstructuredToInfo(obj)
		if err != nil {
			return infos, err
		}
		infos = append(infos, inf)
	}
	return infos, nil
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package info

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/cmd/util"
)

// InfoHelper provides functions for interacting with Info objects.
type InfoHelper interface {
	// UpdateInfos sets the mapping and client for the provided Info
	// objects. This must be called at a time when all needed resource
	// types are available in the RESTMapper.
	UpdateInfos(infos []*resource.Info) error
}

func NewInfoHelper(factory util.Factory) *infoHelper {
	return &infoHelper{
		factory: factory,
	}
}

type infoHelper struct {
	factory util.Factory
}

func (ih *infoHelper) UpdateInfos(infos []*resource.Info) error {
	mapper, err := ih.factory.ToRESTMapper()
	if err != nil {
		return err
	}
	for _, info := range infos {
		gvk := info.Object.GetObjectKind().GroupVersionKind()
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return err
		}
		info.Mapping = mapping

		c, err := ih.getClient(gvk.GroupVersion())
		if err != nil {
			return err
		}
		info.Client = c
	}
	return nil
}

func (ih *infoHelper) ToRESTMapper() (meta.RESTMapper, error) {
	return ih.factory.ToRESTMapper()
}

func (ih *infoHelper) getClient(gv schema.GroupVersion) (*rest.RESTClient, error) {
	cfg, err := ih.factory.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	cfg.ContentConfig = resource.UnstructuredPlusDefaultContentConfig()
	cfg.GroupVersion = &gv
	if len(gv.Group) == 0 {
		cfg.APIPath = "/api"
	} else {
		cfg.APIPath = "/apis"
	}

	return rest.RESTClientFor(cfg)
}

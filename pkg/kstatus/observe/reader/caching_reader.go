// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package reader

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This map is hard-coded knowledge that a Deployment contains and
// ReplicaSet, and that a ReplicaSet in turn contains Pods, etc., and the
// approach to finding status being used here requires hardcoding that
// knowledge in the status client library.
// TODO: These should probably be defined in the observers rather than here.
var genGroupKinds = map[schema.GroupKind][]schema.GroupKind{
	schema.GroupKind{Group: "apps", Kind: "Deployment"}: { //nolint:gofmt
		{
			Group: "apps",
			Kind:  "ReplicaSet",
		},
	},
	schema.GroupKind{Group: "apps", Kind: "ReplicaSet"}: { //nolint:gofmt
		{
			Group: "",
			Kind:  "Pod",
		},
	},
	schema.GroupKind{Group: "apps", Kind: "StatefulSet"}: { //nolint:gofmt
		{
			Group: "",
			Kind:  "Pod",
		},
	},
}

// NewCachingClusterReader returns a new instance of the ObserverReader. The
// ClusterReader needs will use the reader to fetch resources from the cluster,
// while the mapper is used to resolve the version for GroupKinds. The list of
// identifiers is needed so the ClusterReader can figure out which GroupKind
// and namespace combinations it needs to cache when the Sync function is called.
// We only want to fetch the resources that are actually needed.
func NewCachingClusterReader(reader client.Reader, mapper meta.RESTMapper, identifiers []wait.ResourceIdentifier) (*CachingClusterReader, error) {
	gvkNamespaceSet := newGnSet()
	for _, id := range identifiers {
		// For every identifier, add the GroupVersionKind and namespace combination to the gvkNamespaceSet and
		// check the genGroupKinds map for any generated resources that also should be included.
		err := buildGvkNamespaceSet(mapper, []schema.GroupKind{id.GroupKind}, id.Namespace, gvkNamespaceSet)
		if err != nil {
			return nil, err
		}
	}

	return &CachingClusterReader{
		reader: reader,
		mapper: mapper,
		gns:    gvkNamespaceSet.gvkNamespaces,
	}, nil
}

func buildGvkNamespaceSet(mapper meta.RESTMapper, gks []schema.GroupKind, namespace string, gvkNamespaceSet *gvkNamespaceSet) error {
	for _, gk := range gks {
		mapping, err := mapper.RESTMapping(gk)
		if err != nil {
			return err
		}
		gvkNamespaceSet.add(gvkNamespace{
			GVK:       mapping.GroupVersionKind,
			Namespace: namespace,
		})
		genGKs, found := genGroupKinds[gk]
		if found {
			err := buildGvkNamespaceSet(mapper, genGKs, namespace, gvkNamespaceSet)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type gvkNamespaceSet struct {
	gvkNamespaces []gvkNamespace
	seen          map[gvkNamespace]bool
}

func newGnSet() *gvkNamespaceSet {
	return &gvkNamespaceSet{
		gvkNamespaces: make([]gvkNamespace, 0),
		seen:          make(map[gvkNamespace]bool),
	}
}

func (g *gvkNamespaceSet) add(gn gvkNamespace) {
	if _, found := g.seen[gn]; !found {
		g.gvkNamespaces = append(g.gvkNamespaces, gn)
		g.seen[gn] = true
	}
}

// CachingClusterReader is an implementation of the ObserverReader interface that will
// pre-fetch all resources needed before every sync loop. The resources needed are decided by
// finding all combinations of GroupVersionKind and namespace referenced by the provided
// identifiers. This list is then expanded to include any known generated resource types.
type CachingClusterReader struct {
	sync.RWMutex

	// reader provides functions to read and list resources from the
	// cluster.
	reader client.Reader

	// mapper is the client-side representation of the server-side scheme. It is used
	// to resolve GroupVersionKind from GroupKind.
	mapper meta.RESTMapper

	// gns contains the slice of all the GVK and namespace combinations that
	// should be included in the cache. This is computed based the resource identifiers
	// passed in when the CachingClusterReader is created and augmented with other
	// resource types needed to compute status (see genGroupKinds).
	gns []gvkNamespace

	// cache contains the resources found in the cluster for the given combination
	// of GVK and namespace. Before each polling cycle, the framework will call the
	// Sync function, which is responsible for repopulating the cache.
	cache map[gvkNamespace]unstructured.UnstructuredList
}

// gvkNamespace contains information about a GroupVersionKind and a namespace.
type gvkNamespace struct {
	GVK       schema.GroupVersionKind
	Namespace string
}

// Get looks up the resource identified by the key and the object GVK in the cache. If the needed combination
// of GVK and namespace is not part of the cache, that is considered an error.
func (c *CachingClusterReader) Get(_ context.Context, key client.ObjectKey, obj *unstructured.Unstructured) error {
	c.RLock()
	defer c.RUnlock()
	gvk := obj.GetObjectKind().GroupVersionKind()
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind())
	if err != nil {
		return err
	}
	gn := gvkNamespace{
		GVK:       gvk,
		Namespace: key.Namespace,
	}
	cacheList, found := c.cache[gn]
	if !found {
		return fmt.Errorf("GVK %s and Namespace %s not found in cache", gvk.String(), gn.Namespace)
	}
	for _, u := range cacheList.Items {
		if u.GetName() == key.Name {
			obj.Object = u.Object
			return nil
		}
	}
	return errors.NewNotFound(mapping.Resource.GroupResource(), key.Name)
}

// ListNamespaceScoped lists all resource identifier by the GVK of the list, the namespace and the selector
// from the cache. If the needed combination of GVK and namespace is not part of the cache, that is considered an error.
func (c *CachingClusterReader) ListNamespaceScoped(_ context.Context, list *unstructured.UnstructuredList, namespace string, selector labels.Selector) error {
	c.RLock()
	defer c.RUnlock()
	gvk := list.GroupVersionKind()
	gn := gvkNamespace{
		GVK:       gvk,
		Namespace: namespace,
	}

	cacheList, found := c.cache[gn]
	if !found {
		return fmt.Errorf("GVK %s and Namespace %s not found in cache", gvk.String(), gn.Namespace)
	}

	var items []unstructured.Unstructured
	for _, u := range cacheList.Items {
		if selector.Matches(labels.Set(u.GetLabels())) {
			items = append(items, u)
		}
	}
	list.Items = items
	return nil
}

// ListClusterScoped lists all resource identifier by the GVK of the list and selector
// from the cache. If the needed combination of GVK and namespace (which for clusterscoped resources
// will always be the empty string) is not part of the cache, that is considered an error.
func (c *CachingClusterReader) ListClusterScoped(ctx context.Context, list *unstructured.UnstructuredList, selector labels.Selector) error {
	return c.ListNamespaceScoped(ctx, list, "", selector)
}

// Sync loops over the list of gvkNamespace we know of, and uses list calls to fetch the resources.
// This information populates the cache.
func (c *CachingClusterReader) Sync(ctx context.Context) error {
	c.Lock()
	defer c.Unlock()
	cache := make(map[gvkNamespace]unstructured.UnstructuredList)
	for _, gn := range c.gns {
		mapping, err := c.mapper.RESTMapping(gn.GVK.GroupKind())
		if err != nil {
			return err
		}
		var listOptions []client.ListOption
		if mapping.Scope == meta.RESTScopeNamespace {
			listOptions = append(listOptions, client.InNamespace(gn.Namespace))
		}
		var list unstructured.UnstructuredList
		list.SetGroupVersionKind(gn.GVK)
		err = c.reader.List(ctx, &list, listOptions...)
		if err != nil {
			return err
		}
		cache[gn] = list
	}
	c.cache = cache
	return nil
}

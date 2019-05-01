/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/dynamic"
)

// NewForConfig returns a new Client using the provided config and Options.
// The returned client reads *and* writes directly from the server
// (it doesn't use object caches).  It understands how to work with
// normal types (both custom resources and aggregated/built-in resources),
// as well as unstructured types.
//
// In the case of normal types, the scheme will be used to look up the
// corresponding group, version, and kind for the given type.  In the
// case of unstrctured types, the group, version, and kind will be extracted
// from the corresponding fields on the object.
func NewForConfig(dynamicClient dynamic.Interface, mapper meta.RESTMapper) (Client, error) {
	c := &client{
		client:     dynamicClient,
		restMapper: mapper,
	}

	return c, nil
}

var _ Client = &client{}

// client is a client.Client that reads and writes directly from/to an API server.  It lazily initializes
// new clients at the time they are used, and caches the client.
type client struct {
	client     dynamic.Interface
	restMapper meta.RESTMapper
}

// Patch object
type Patch struct {
	Type types.PatchType
	Data []byte
}

// Create Creates an object using dynamic client
func (uc *client) Create(_ context.Context, obj runtime.Object, options metav1.CreateOptions) error {
	u, r, err := uc.getUnstructuredResource(obj, "")
	if err != nil {
		return err
	}
	i, err := r.Create(u, options)
	if err != nil {
		return err
	}
	u.Object = i.Object
	return nil
}

// Update updates an object using dynamic client
func (uc *client) Update(_ context.Context, obj runtime.Object, options metav1.UpdateOptions) error {
	u, r, err := uc.getUnstructuredResource(obj, "")
	if err != nil {
		return err
	}
	i, err := r.Update(u, options)
	if err != nil {
		return err
	}
	u.Object = i.Object
	return nil
}

// Delete calls the delete of an object using dynamic client
func (uc *client) Delete(_ context.Context, obj runtime.Object, options *metav1.DeleteOptions) error {
	u, r, err := uc.getUnstructuredResource(obj, "")
	if err != nil {
		return err
	}
	err = r.Delete(u.GetName(), options)
	return err
}

// Patch implements patching of an object
func (uc *client) Patch(_ context.Context, obj runtime.Object, patch Patch, options *metav1.PatchOptions) error {
	u, r, err := uc.getUnstructuredResource(obj, "")
	if err != nil {
		return err
	}

	i, err := r.Patch(u.GetName(), patch.Type, patch.Data, *options)
	if err != nil {
		return err
	}
	u.Object = i.Object
	return nil
}

// Get fetches the requested object into the input obj using dynamic client
func (uc *client) Get(_ context.Context, key types.NamespacedName, obj runtime.Object) error {
	u, r, err := uc.getUnstructuredResource(obj, key.Namespace)
	if err != nil {
		return err
	}
	i, err := r.Get(key.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	u.Object = i.Object
	return nil
}

// List fetches the list of objects into the input obj using dynamic client
func (uc *client) List(_ context.Context, obj runtime.Object, namespace string, options metav1.ListOptions) error {
	u, ok := obj.(*unstructured.UnstructuredList)
	if !ok {
		return fmt.Errorf("unstructured client did not understand object: %T", obj)
	}
	gvk := u.GroupVersionKind()
	if strings.HasSuffix(gvk.Kind, "List") {
		gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	}
	r, err := uc.getResourceInterface(gvk, namespace)
	if err != nil {
		return err
	}

	i, err := r.List(options)
	if err != nil {
		return err
	}
	u.Items = i.Items
	u.Object = i.Object
	return nil
}

// UpdateStatus updates the status subresource using dynamic client
func (uc *client) UpdateStatus(_ context.Context, obj runtime.Object) error {
	u, r, err := uc.getUnstructuredResource(obj, "")
	if err != nil {
		return err
	}
	i, err := r.UpdateStatus(u, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	u.Object = i.Object
	return nil
}

// Apply - use merge patch to apply an object
func (uc *client) Apply(c context.Context, obj runtime.Object) error {

	u, r, err := uc.getUnstructuredResource(obj, "")
	if err != nil {
		return err
	}
	i, err := r.Get(u.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	patch, err := MergePatch(i, obj)
	if err != nil {
		return err
	}

	return uc.Patch(c, obj, patch, nil)
}

func (uc *client) getResourceInterface(gvk schema.GroupVersionKind, ns string) (dynamic.ResourceInterface, error) {
	mapping, err := uc.restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		return uc.client.Resource(mapping.Resource), nil
	}
	return uc.client.Resource(mapping.Resource).Namespace(ns), nil
}

func (uc *client) getUnstructuredResource(obj runtime.Object, namespace string) (*unstructured.Unstructured, dynamic.ResourceInterface, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, nil, fmt.Errorf("unstructured client did not understand object: %T", obj)
	}
	if namespace == "" {
		namespace = u.GetNamespace()
	}
	r, err := uc.getResourceInterface(u.GroupVersionKind(), namespace)
	if err != nil {
		return nil, nil, err
	}

	return u, r, nil
}

// MergePatch - generate merge patch from original and modified objects
func MergePatch(original, modified runtime.Object) (Patch, error) {
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return Patch{}, err
	}

	modifiedJSON, err := json.Marshal(modified)
	if err != nil {
		return Patch{}, err
	}

	patch, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return Patch{}, err
	}
	return Patch{types.MergePatchType, patch}, nil
}

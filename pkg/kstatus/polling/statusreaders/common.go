// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// BaseStatusReader provides some basic functionality needed by the statusreaders.
type BaseStatusReader struct {
	Reader engine.ClusterReader

	Mapper meta.RESTMapper

	computeStatusFunc engine.ComputeStatusFunc
}

// SetComputeStatusFunc allows for setting the function used by the engine for computing status. The default
// value here is to use the status package. This is provided for testing purposes.
func (b *BaseStatusReader) SetComputeStatusFunc(statusFunc engine.ComputeStatusFunc) {
	b.computeStatusFunc = statusFunc
}

// LookupResource looks up a resource with the given identifier. It will use the rest mapper to resolve
// the version of the GroupKind given in the identifier.
// If the resource is found, it is returned. If it is not found or something
// went wrong, the function will return an error.
func (b *BaseStatusReader) LookupResource(ctx context.Context, identifier prune.ObjMetadata) (*unstructured.Unstructured, error) {
	GVK, err := b.GVK(identifier.GroupKind)
	if err != nil {
		return nil, err
	}

	var u unstructured.Unstructured
	u.SetGroupVersionKind(GVK)
	key := keyForNamespacedResource(identifier)
	err = b.Reader.Get(ctx, key, &u)
	if err != nil {
		return nil, err
	}
	u.SetNamespace(identifier.Namespace)
	return &u, nil
}

// StatusForGeneratedResources provides a way to fetch the statuses for all resources of a given GroupKind
// that match the selector in the provided resource. Typically, this is used to fetch the status of generated
// resources.
func (b *BaseStatusReader) StatusForGeneratedResources(ctx context.Context, statusReader engine.StatusReader, object *unstructured.Unstructured,
	gk schema.GroupKind, selectorPath ...string) (event.ResourceStatuses, error) {
	namespace := getNamespaceForNamespacedResource(object)
	selector, err := toSelector(object, selectorPath...)
	if err != nil {
		return event.ResourceStatuses{}, err
	}

	var objectList unstructured.UnstructuredList
	gvk, err := b.GVK(gk)
	if err != nil {
		return event.ResourceStatuses{}, err
	}
	objectList.SetGroupVersionKind(gvk)
	err = b.Reader.ListNamespaceScoped(ctx, &objectList, namespace, selector)
	if err != nil {
		return event.ResourceStatuses{}, err
	}

	var resourceStatuses event.ResourceStatuses
	for i := range objectList.Items {
		generatedObject := objectList.Items[i]
		resourceStatus := statusReader.ReadStatusForObject(ctx, &generatedObject)
		resourceStatuses = append(resourceStatuses, resourceStatus)
	}
	sort.Sort(resourceStatuses)
	return resourceStatuses, nil
}

// handleResourceStatusError construct the appropriate ResourceStatus
// object based on the type of error.
func (b *BaseStatusReader) handleResourceStatusError(identifier prune.ObjMetadata, err error) *event.ResourceStatus {
	if errors.IsNotFound(err) {
		return &event.ResourceStatus{
			Identifier: identifier,
			Status:     status.NotFoundStatus,
			Message:    "Resource not found",
		}
	}
	return &event.ResourceStatus{
		Identifier: identifier,
		Status:     status.UnknownStatus,
		Error:      err,
	}
}

// GVK looks up the GVK from a GroupKind using the rest mapper.
func (b *BaseStatusReader) GVK(gk schema.GroupKind) (schema.GroupVersionKind, error) {
	mapping, err := b.Mapper.RESTMapping(gk)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return mapping.GroupVersionKind, nil
}

func toSelector(resource *unstructured.Unstructured, path ...string) (labels.Selector, error) {
	selector, found, err := unstructured.NestedMap(resource.Object, path...)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no selector found")
	}
	bytes, err := json.Marshal(selector)
	if err != nil {
		return nil, err
	}
	var s metav1.LabelSelector
	err = json.Unmarshal(bytes, &s)
	if err != nil {
		return nil, err
	}
	return metav1.LabelSelectorAsSelector(&s)
}

func toIdentifier(u *unstructured.Unstructured) prune.ObjMetadata {
	return prune.ObjMetadata{
		GroupKind: u.GroupVersionKind().GroupKind(),
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}
}

// getNamespaceForNamespacedResource returns the namespace for the given object,
// but includes logic for returning the default namespace if it is not set.
func getNamespaceForNamespacedResource(object runtime.Object) string {
	acc, err := meta.Accessor(object)
	if err != nil {
		panic(err)
	}
	ns := acc.GetNamespace()
	if ns == "" {
		return "default"
	}
	return ns
}

// keyForNamespacedResource returns the object key for the given identifier. It makes
// sure to set the namespace to default if it is not provided.
func keyForNamespacedResource(identifier prune.ObjMetadata) types.NamespacedName {
	namespace := "default"
	if identifier.Namespace != "" {
		namespace = identifier.Namespace
	}
	return types.NamespacedName{
		Name:      identifier.Name,
		Namespace: namespace,
	}
}

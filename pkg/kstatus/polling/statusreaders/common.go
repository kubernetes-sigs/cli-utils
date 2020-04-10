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
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// baseStatusReader is the implementation of the StatusReader interface defined
// in the engine package. It contains the basic logic needed for every resource.
// In order to handle resource specific logic, it must include an implementation
// of the resourceTypeStatusReader interface.
// In practice we will create many instances of baseStatusReader, each with a different
// implementation of the resourceTypeStatusReader interface and therefore each
// of the instances will be able to handle different resource types.
type baseStatusReader struct {
	// reader is an implementation of the ClusterReader interface. It provides a
	// way for the StatusReader to fetch resources from the cluster.
	reader engine.ClusterReader

	// mapper provides a way to look up the resource types that are available
	// in the cluster.
	mapper meta.RESTMapper

	// resourceStatusReader is an resource-type specific implementation
	// of the resourceTypeStatusReader interface. While the baseStatusReader
	// contains the logic shared between all resource types, this implementation
	// will contain the resource specific info.
	resourceStatusReader resourceTypeStatusReader
}

// resourceTypeStatusReader is an interface that can be implemented differently
// for each resource type.
type resourceTypeStatusReader interface {
	ReadStatusForObject(ctx context.Context, object *unstructured.Unstructured) *event.ResourceStatus
}

// ReadStatus reads the object identified by the passed-in identifier and computes it's status. It reads
// the resource here, but computing status is delegated to the ReadStatusForObject function.
func (b *baseStatusReader) ReadStatus(ctx context.Context, identifier object.ObjMetadata) *event.ResourceStatus {
	object, err := b.lookupResource(ctx, identifier)
	if err != nil {
		return handleResourceStatusError(identifier, err)
	}
	return b.resourceStatusReader.ReadStatusForObject(ctx, object)
}

// ReadStatusForObject computes the status for the passed-in object. Since this is specific for each
// resource type, the actual work is delegated to the implementation of the resourceTypeStatusReader interface.
func (b *baseStatusReader) ReadStatusForObject(ctx context.Context, object *unstructured.Unstructured) *event.ResourceStatus {
	return b.resourceStatusReader.ReadStatusForObject(ctx, object)
}

// lookupResource looks up a resource with the given identifier. It will use the rest mapper to resolve
// the version of the GroupKind given in the identifier.
// If the resource is found, it is returned. If it is not found or something
// went wrong, the function will return an error.
func (b *baseStatusReader) lookupResource(ctx context.Context, identifier object.ObjMetadata) (*unstructured.Unstructured, error) {
	GVK, err := gvk(identifier.GroupKind, b.mapper)
	if err != nil {
		return nil, err
	}

	var u unstructured.Unstructured
	u.SetGroupVersionKind(GVK)
	key := types.NamespacedName{
		Name:      identifier.Name,
		Namespace: identifier.Namespace,
	}
	err = b.reader.Get(ctx, key, &u)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// statusForGenResourcesFunc defines the function type used by the statusForGeneratedResource function.
// TODO: Find a better solution for this. Maybe put the logic for looking up generated resources
// into a separate type.
type statusForGenResourcesFunc func(ctx context.Context, mapper meta.RESTMapper, reader engine.ClusterReader, statusReader resourceTypeStatusReader,
	object *unstructured.Unstructured, gk schema.GroupKind, selectorPath ...string) (event.ResourceStatuses, error)

// statusForGeneratedResources provides a way to fetch the statuses for all resources of a given GroupKind
// that match the selector in the provided resource. Typically, this is used to fetch the status of generated
// resources.
func statusForGeneratedResources(ctx context.Context, mapper meta.RESTMapper, reader engine.ClusterReader, statusReader resourceTypeStatusReader,
	object *unstructured.Unstructured, gk schema.GroupKind, selectorPath ...string) (event.ResourceStatuses, error) {
	namespace := getNamespaceForNamespacedResource(object)
	selector, err := toSelector(object, selectorPath...)
	if err != nil {
		return event.ResourceStatuses{}, err
	}

	var objectList unstructured.UnstructuredList
	gvk, err := gvk(gk, mapper)
	if err != nil {
		return event.ResourceStatuses{}, err
	}
	objectList.SetGroupVersionKind(gvk)
	err = reader.ListNamespaceScoped(ctx, &objectList, namespace, selector)
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
func handleResourceStatusError(identifier object.ObjMetadata, err error) *event.ResourceStatus {
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

// gvk looks up the GVK from a GroupKind using the rest mapper.
func gvk(gk schema.GroupKind, mapper meta.RESTMapper) (schema.GroupVersionKind, error) {
	mapping, err := mapper.RESTMapping(gk)
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

func toIdentifier(u *unstructured.Unstructured) object.ObjMetadata {
	return object.ObjMetadata{
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
	return acc.GetNamespace()
}

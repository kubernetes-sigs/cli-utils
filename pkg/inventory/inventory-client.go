// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/validation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// InventoryClient expresses an interface for retrieving Inventory
// objects in the clus
type InventoryClient interface {
	// GetPreviousInventoryObjects returns the inventory objects (as *resource.Info)
	// that are currently stored in the cluster, or an error if one occurred. Uses
	// information from the current (recently) created inventory object to find
	// the previously applied inventory object in the cluster.
	GetPreviousInventoryObjects(currentInv *resource.Info) ([]*resource.Info, error)
	// GetStoredObjRefs returns the set of previously applied objects as ObjMetadata,
	// or an error if one occurred. This set of previously applied object references
	// is stored in the inventory objects living in the cluster. Uses information
	// from the current inventory object to find the previously applied inventory
	// objects.
	GetStoredObjRefs(currentInv *resource.Info) ([]object.ObjMetadata, error)
}

// FakeInventoryClient is a testing implementation of the InventoryClient interface.
type FakeInventoryClient struct {
	prevInventories []*resource.Info
}

var _ InventoryClient = &FakeInventoryClient{}

// NewFakeInventoryClient returns a FakeInventoryClient.
func NewFakeInventoryClient(prevInventories []*resource.Info) *FakeInventoryClient {
	return &FakeInventoryClient{prevInventories: prevInventories}
}

// GetPreviousInventoryObjects returns the hard-coded set of resource infos.
// This function ensures the fake implements the InventoryClient interface.
func (fic *FakeInventoryClient) GetPreviousInventoryObjects(currentInv *resource.Info) ([]*resource.Info, error) {
	return fic.prevInventories, nil
}

// GetStoredObjRefs returns the union of hard-coded object references stored
// in the prevInventories.
func (fic *FakeInventoryClient) GetStoredObjRefs(currentInv *resource.Info) ([]object.ObjMetadata, error) {
	return UnionPastObjs(fic.prevInventories)
}

// ClusterInventoryClient is a concrete implementation of the
// InventoryClient interface.
type ClusterInventoryClient struct {
	builder                   *resource.Builder
	mapper                    meta.RESTMapper
	validator                 validation.Schema
	pastInventoryObjects      []*resource.Info
	retrievedInventoryObjects bool
}

var _ InventoryClient = &ClusterInventoryClient{}

// NewInventoryClient returns a concrete implementation of the
// InventoryClient interface or an error.
func NewInventoryClient(factory util.Factory) (*ClusterInventoryClient, error) {
	var err error
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	validator, err := factory.Validator(false)
	if err != nil {
		return nil, err
	}
	builder := factory.NewBuilder()
	clusterInventoryClient := ClusterInventoryClient{
		builder:                   builder,
		mapper:                    mapper,
		validator:                 validator,
		pastInventoryObjects:      []*resource.Info{},
		retrievedInventoryObjects: false,
	}
	return &clusterInventoryClient, nil
}

// GetStoredObjRefs returns the set of previously applied objects as ObjMetadata,
// or an error if one occurred. This set of previously applied object references
// is stored in the inventory objects living in the cluster. Uses information
// from the current inventory object to find the previously applied inventory
// objects.
func (cic *ClusterInventoryClient) GetStoredObjRefs(currentInv *resource.Info) ([]object.ObjMetadata, error) {
	prevInventories, err := cic.GetPreviousInventoryObjects(currentInv)
	if err != nil {
		return nil, err
	}
	return UnionPastObjs(prevInventories)
}

// GetPreviousInventoryObjects returns the set of inventory objects
// that have the same label as the current inventory object. Removes
// the current inventory object from this set. Returns an error
// if there is a problem retrieving the inventory objects.
func (cic *ClusterInventoryClient) GetPreviousInventoryObjects(currentInv *resource.Info) ([]*resource.Info, error) {
	current, err := infoToObjMetadata(currentInv)
	if err != nil {
		return nil, err
	}
	label, err := retrieveInventoryLabel(currentInv.Object)
	if err != nil {
		return nil, err
	}
	prevInventoryObjs, err := cic.retrievePreviousInventoryObjects(current, label)
	if err != nil {
		return nil, err
	}
	// Remove the current inventory info from the previous inventory infos.
	pastInventoryInfos := []*resource.Info{}
	for _, pastInfo := range prevInventoryObjs {
		past, err := infoToObjMetadata(pastInfo)
		if err != nil {
			return nil, err
		}
		if !current.Equals(past) {
			pastInventoryInfos = append(pastInventoryInfos, pastInfo)
		}
	}
	return pastInventoryInfos, nil
}

// retrievePreviousInventoryObjects requests the previous inventory objects
// using the inventory label from the current inventory object. Sets
// the field "pastInventoryObjects". Returns an error if the inventory
// label doesn't exist for the current currentInventoryObject does not
// exist or if the call to retrieve the past inventory objects fails.
func (cic *ClusterInventoryClient) retrievePreviousInventoryObjects(current *object.ObjMetadata, label string) ([]*resource.Info, error) {
	if cic.retrievedInventoryObjects {
		return cic.pastInventoryObjects, nil
	}
	mapping, err := cic.mapper.RESTMapping(current.GroupKind)
	if err != nil {
		return nil, err
	}
	groupResource := mapping.Resource.GroupResource().String()
	namespace := current.Namespace
	labelSelector := fmt.Sprintf("%s=%s", common.InventoryLabel, label)
	klog.V(4).Infof("prune inventory object fetch: %s/%s/%s", groupResource, namespace, labelSelector)
	retrievedInventoryInfos, err := cic.builder.
		Unstructured().
		// TODO: Check if this validator is necessary.
		Schema(cic.validator).
		ContinueOnError().
		NamespaceParam(namespace).DefaultNamespace().
		ResourceTypes(groupResource).
		LabelSelectorParam(labelSelector).
		Flatten().
		Do().
		Infos()
	if err != nil {
		return nil, err
	}
	cic.pastInventoryObjects = retrievedInventoryInfos
	cic.retrievedInventoryObjects = true
	klog.V(4).Infof("prune %d inventory objects found", len(cic.pastInventoryObjects))
	return retrievedInventoryInfos, nil
}

// infoToObjMetadata transforms the object represented by the passed "info"
// into its Inventory representation. Returns error if the passed Info
// is nil, or the Object in the Info is empty.
func infoToObjMetadata(info *resource.Info) (*object.ObjMetadata, error) {
	if info == nil || info.Object == nil {
		return nil, fmt.Errorf("empty resource.Info can not calculate as inventory")
	}
	obj := info.Object
	gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
	return object.CreateObjMetadata(info.Namespace, info.Name, gk)
}

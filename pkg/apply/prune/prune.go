// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Prune functionality deletes previously applied objects
// which are subsequently omitted in further apply operations.
// This functionality relies on "inventory" objects to store
// object metadata for each apply operation. This file defines
// PruneOptions to encapsulate information necessary to
// calculate the prune set, and to delete the objects in
// this prune set.

package prune

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/ordering"
)

// PruneOptions encapsulates the necessary information to
// implement the prune functionality.
type PruneOptions struct {
	InvClient inventory.InventoryClient
	Client    dynamic.Interface
	Mapper    meta.RESTMapper
	// True if we are destroying, which deletes the inventory object
	// as well (possibly) the inventory namespace.
	Destroy bool
	// TODO(seans): Replace this with Filter interface to generalize prune skipping.
	LocalNamespaces sets.String
}

// NewPruneOptions returns a struct (PruneOptions) encapsulating the necessary
// information to run the prune. Returns an error if an error occurs
// gathering this information.
func NewPruneOptions() *PruneOptions {
	po := &PruneOptions{
		Destroy:         false,
		LocalNamespaces: sets.NewString(),
	}
	return po
}

func (po *PruneOptions) Initialize(factory util.Factory, invClient inventory.InventoryClient) error {
	var err error
	// Client/Builder fields from the Factory.
	po.Client, err = factory.DynamicClient()
	if err != nil {
		return err
	}
	po.Mapper, err = factory.ToRESTMapper()
	if err != nil {
		return err
	}
	po.InvClient = invClient
	return nil
}

// Options defines a set of parameters that can be used to tune
// the behavior of the pruner.
type Options struct {
	// DryRunStrategy defines whether objects should actually be pruned or if
	// we should just print what would happen without actually doing it.
	DryRunStrategy common.DryRunStrategy

	PropagationPolicy metav1.DeletionPropagation

	// InventoryPolicy defines the inventory policy of prune.
	InventoryPolicy inventory.InventoryPolicy
}

// Prune deletes the set of passed pruneObjs.
//
// Parameters:
//   localInv - locally read inventory object
//   pruneObjs - objects to prune (delete)
//   currentUIDs - UIDs for successfully applied objects
//   taskContext - task for apply/prune
func (po *PruneOptions) Prune(localInv inventory.InventoryInfo,
	pruneObjs []*unstructured.Unstructured,
	currentUIDs sets.String,
	taskContext *taskrunner.TaskContext,
	o Options) error {
	// Validate parameters
	if localInv == nil {
		return fmt.Errorf("the local inventory object can't be nil")
	}
	// Sort the resources in reverse order using the same rules as is
	// used for apply.
	eventFactory := CreateEventFactory(po.Destroy)
	for _, pruneObj := range pruneObjs {
		pruneID := object.UnstructuredToObjMeta(pruneObj)
		klog.V(5).Infof("attempting prune: %s", pruneID)
		// Do not prune objects that are in set of currently applied objects.
		uid := string(pruneObj.GetUID())
		if currentUIDs.Has(uid) {
			klog.V(5).Infof("prune object in current apply; do not prune: %s", uid)
			continue
		}
		// Handle lifecycle directive preventing deletion.
		if !canPrune(localInv, pruneObj, o.InventoryPolicy, uid) {
			klog.V(4).Infof("skip prune for lifecycle directive %s", pruneID)
			taskContext.EventChannel() <- eventFactory.CreateSkippedEvent(pruneObj)
			taskContext.CapturePruneFailure(pruneID)
			continue
		}
		// If regular pruning (not destroying), skip deleting namespace containing
		// currently applied objects.
		if pruneID.GroupKind == object.CoreV1Namespace.GroupKind() &&
			po.LocalNamespaces.Has(pruneID.Name) {
			klog.V(4).Infof("skip pruning namespace: %s", pruneID.Name)
			taskContext.EventChannel() <- eventFactory.CreateSkippedEvent(pruneObj)
			taskContext.CapturePruneFailure(pruneID)
			continue
		}
		if !o.DryRunStrategy.ClientOrServerDryRun() {
			klog.V(4).Infof("prune object delete: %s", pruneID)
			namespacedClient, err := po.namespacedClient(pruneID)
			if err != nil {
				if klog.V(4).Enabled() {
					klog.Errorf("prune failed for %s (%s)", pruneID, err)
				}
				taskContext.EventChannel() <- eventFactory.CreateFailedEvent(pruneID, err)
				taskContext.CapturePruneFailure(pruneID)
				continue
			}
			err = namespacedClient.Delete(context.TODO(), pruneID.Name, metav1.DeleteOptions{})
			if err != nil {
				if klog.V(4).Enabled() {
					klog.Errorf("prune failed for %s (%s)", pruneID, err)
				}
				taskContext.EventChannel() <- eventFactory.CreateFailedEvent(pruneID, err)
				taskContext.CapturePruneFailure(pruneID)
				continue
			}
		}
		taskContext.EventChannel() <- eventFactory.CreateSuccessEvent(pruneObj)
	}
	return nil
}

// GetPruneObjs calculates the set of prune objects, and retrieves them
// from the cluster. Set of prune objects equals the set of inventory
// objects minus the set of currently applied objects. Returns an error
// if one occurs.
func (po *PruneOptions) GetPruneObjs(inv inventory.InventoryInfo,
	localObjs []*unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	localIds := object.UnstructuredsToObjMetas(localObjs)
	prevInvIds, err := po.InvClient.GetClusterObjs(inv)
	if err != nil {
		return nil, err
	}
	pruneIds := object.SetDiff(prevInvIds, localIds)
	pruneObjs := []*unstructured.Unstructured{}
	for _, pruneID := range pruneIds {
		pruneObj, err := po.GetObject(pruneID)
		if err != nil {
			return nil, err
		}
		pruneObjs = append(pruneObjs, pruneObj)
	}
	sort.Sort(sort.Reverse(ordering.SortableUnstructureds(pruneObjs)))
	return pruneObjs, nil
}

// GetObject uses the passed object data to retrieve the object
// from the cluster (or an error if one occurs).
func (po *PruneOptions) GetObject(obj object.ObjMetadata) (*unstructured.Unstructured, error) {
	namespacedClient, err := po.namespacedClient(obj)
	if err != nil {
		return nil, err
	}
	return namespacedClient.Get(context.TODO(), obj.Name, metav1.GetOptions{})
}

func (po *PruneOptions) namespacedClient(obj object.ObjMetadata) (dynamic.ResourceInterface, error) {
	mapping, err := po.Mapper.RESTMapping(obj.GroupKind)
	if err != nil {
		return nil, err
	}
	return po.Client.Resource(mapping.Resource).Namespace(obj.Namespace), nil
}

// preventDeleteAnnotation returns true if the "onRemove:keep"
// annotation exists within the annotation map; false otherwise.
func preventDeleteAnnotation(annotations map[string]string) bool {
	for annotation, value := range annotations {
		if common.NoDeletion(annotation, value) {
			return true
		}
	}
	return false
}

func canPrune(localInv inventory.InventoryInfo, obj *unstructured.Unstructured,
	policy inventory.InventoryPolicy, uid string) bool {
	if !inventory.CanPrune(localInv, obj, policy) {
		klog.V(4).Infof("skip pruning object that doesn't belong to current inventory: %s", uid)
		return false
	}
	if preventDeleteAnnotation(obj.GetAnnotations()) {
		klog.V(4).Infof("prune object lifecycle directive; do not prune: %s", uid)
		return false
	}
	return true
}

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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/ordering"
)

// PruneOptions encapsulates the necessary information to
// implement the prune functionality.
type PruneOptions struct {
	InvClient inventory.InventoryClient
	client    dynamic.Interface
	mapper    meta.RESTMapper
	// True if we are destroying, which deletes the inventory object
	// as well (possibly) the inventory namespace.
	Destroy bool
}

// NewPruneOptions returns a struct (PruneOptions) encapsulating the necessary
// information to run the prune. Returns an error if an error occurs
// gathering this information.
func NewPruneOptions() *PruneOptions {
	po := &PruneOptions{
		Destroy: false,
	}
	return po
}

func (po *PruneOptions) Initialize(factory util.Factory, invClient inventory.InventoryClient) error {
	var err error
	// Client/Builder fields from the Factory.
	po.client, err = factory.DynamicClient()
	if err != nil {
		return err
	}
	po.mapper, err = factory.ToRESTMapper()
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

// Prune deletes the set of resources which were previously applied
// (retrieved from previous inventory objects) but omitted in
// the current apply. Prune also delete all previous inventory
// objects. Returns an error if there was a problem.
func (po *PruneOptions) Prune(localInv inventory.InventoryInfo, localObjs []*unstructured.Unstructured, currentUIDs sets.String,
	eventChannel chan<- event.Event, o Options) error {
	if localInv == nil {
		return fmt.Errorf("the local inventory object can't be nil")
	}
	invNamespace := localInv.Namespace()
	klog.V(4).Infof("prune local inventory object: %s/%s", invNamespace, localInv.Name())
	// Get the list of Object Meta from the local objects.
	localIds := object.UnstructuredsToObjMetas(localObjs)
	// Create the set of namespaces for currently (locally) applied objects, including
	// the namespace the inventory object lives in (if it's not cluster-scoped). When
	// pruning, check this set of namespaces to ensure these namespaces are not deleted.
	localNamespaces := mergeObjNamespaces(localObjs)
	if invNamespace != "" {
		localNamespaces.Insert(invNamespace)
	}
	clusterObjs, err := po.InvClient.GetClusterObjs(localInv)
	if err != nil {
		return err
	}
	klog.V(4).Infof("prune %d currently applied objects", len(currentUIDs))
	klog.V(4).Infof("prune %d previously applied objects", len(clusterObjs))
	// Sort the resources in reverse order using the same rules as is
	// used for apply.
	sort.Sort(sort.Reverse(ordering.SortableMetas(clusterObjs)))
	for _, clusterObj := range clusterObjs {
		mapping, err := po.mapper.RESTMapping(clusterObj.GroupKind)
		if err != nil {
			localIds = append(localIds, clusterObj)
			eventChannel <- createPruneFailedEvent(clusterObj, err)
			continue
		}
		namespacedClient := po.client.Resource(mapping.Resource).Namespace(clusterObj.Namespace)
		obj, err := namespacedClient.Get(context.TODO(), clusterObj.Name, metav1.GetOptions{})
		if err != nil {
			// Object not found in cluster, so no need to delete it; skip to next object.
			if apierrors.IsNotFound(err) {
				continue
			}
			localIds = append(localIds, clusterObj)
			eventChannel <- createPruneFailedEvent(clusterObj, err)
			continue
		}
		metadata, err := meta.Accessor(obj)
		if err != nil {
			localIds = append(localIds, clusterObj)
			eventChannel <- createPruneFailedEvent(clusterObj, err)
			continue
		}
		// If this cluster object is not also a currently applied
		// object, then it has been omitted--prune it. If the cluster
		// object is part of the local apply set, skip it.
		uid := string(metadata.GetUID())
		klog.V(7).Infof("prune previously applied object UID: %s", uid)
		if currentUIDs.Has(uid) {
			klog.V(7).Infof("prune object in current apply; do not prune: %s", uid)
			continue
		}
		// Handle lifecycle directive preventing deletion.
		if !canPrune(localInv, obj, o.InventoryPolicy, uid) {
			eventChannel <- createPruneEvent(clusterObj, obj, event.PruneSkipped)
			localIds = append(localIds, clusterObj)
			continue
		}
		// If regular pruning (not destroying), skip deleting namespace containing
		// currently applied objects.
		if !po.Destroy {
			if clusterObj.GroupKind == object.CoreV1Namespace.GroupKind() &&
				localNamespaces.Has(clusterObj.Name) {
				klog.V(4).Infof("skip pruning namespace: %s", clusterObj.Name)
				eventChannel <- createPruneEvent(clusterObj, obj, event.PruneSkipped)
				localIds = append(localIds, clusterObj)
				continue
			}
		}
		if !o.DryRunStrategy.ClientOrServerDryRun() {
			klog.V(4).Infof("prune object delete: %s/%s", clusterObj.Namespace, clusterObj.Name)
			err = namespacedClient.Delete(context.TODO(), clusterObj.Name, metav1.DeleteOptions{})
			if err != nil {
				eventChannel <- createPruneFailedEvent(clusterObj, err)
				localIds = append(localIds, clusterObj)
				continue
			}
		}
		eventChannel <- createPruneEvent(clusterObj, obj, event.Pruned)
	}
	return po.InvClient.Replace(localInv, localIds)
}

// mergeObjNamespaces returns a set of strings of all the namespaces
// for non cluster-scoped objects. These namespaces are forced to
// lower-case.
func mergeObjNamespaces(objs []*unstructured.Unstructured) sets.String {
	namespaces := sets.NewString()
	for _, obj := range objs {
		namespace := strings.TrimSpace(strings.ToLower(obj.GetNamespace()))
		if namespace != "" {
			namespaces.Insert(namespace)
		}
	}
	return namespaces
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

// createPruneEvent is a helper function to package a prune event.
func createPruneEvent(id object.ObjMetadata, obj *unstructured.Unstructured, op event.PruneEventOperation) event.Event {
	return event.Event{
		Type: event.PruneType,
		PruneEvent: event.PruneEvent{
			Type:       event.PruneEventResourceUpdate,
			Operation:  op,
			Object:     obj,
			Identifier: id,
		},
	}
}

// createPruneEvent is a helper function to package a prune event for a failure.
func createPruneFailedEvent(objMeta object.ObjMetadata, err error) event.Event {
	return event.Event{
		Type: event.PruneType,
		PruneEvent: event.PruneEvent{
			Type:       event.PruneEventFailed,
			Identifier: objMeta,
			Error:      err,
		},
	}
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

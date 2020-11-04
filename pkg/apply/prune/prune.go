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
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
}

// Prune deletes the set of resources which were previously applied
// (retrieved from previous inventory objects) but omitted in
// the current apply. Prune also delete all previous inventory
// objects. Returns an error if there was a problem.
func (po *PruneOptions) Prune(localInv *unstructured.Unstructured, localObjs []*unstructured.Unstructured, currentUIDs sets.String,
	eventChannel chan<- event.Event, o Options) error {
	invNamespace := localInv.GetNamespace()
	klog.V(4).Infof("prune local inventory object: %s/%s", invNamespace, localInv.GetName())
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
			return err
		}
		namespacedClient := po.client.Resource(mapping.Resource).Namespace(clusterObj.Namespace)
		obj, err := namespacedClient.Get(clusterObj.Name, metav1.GetOptions{})
		if err != nil {
			// Object not found in cluster, so no need to delete it; skip to next object.
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		metadata, err := meta.Accessor(obj)
		if err != nil {
			return err
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
		if preventDeleteAnnotation(metadata.GetAnnotations()) {
			klog.V(7).Infof("prune object lifecycle directive; do not prune: %s", uid)
			eventChannel <- createPruneEvent(obj, event.PruneSkipped)
			continue
		}
		// If regular pruning (not destroying), skip deleting inventory namespace.
		if !po.Destroy {
			if clusterObj.GroupKind == object.CoreV1Namespace.GroupKind() &&
				clusterObj.Name == invNamespace {
				klog.V(7).Infof("skip pruning inventory namespace: %s", obj)
				eventChannel <- createPruneEvent(obj, event.PruneSkipped)
				continue
			}
		}
		if !o.DryRunStrategy.ClientOrServerDryRun() {
			klog.V(4).Infof("prune object delete: %s/%s", clusterObj.Namespace, clusterObj.Name)
			err = namespacedClient.Delete(clusterObj.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
		eventChannel <- createPruneEvent(obj, event.Pruned)
	}
	localIds := object.UnstructuredsToObjMetas(localObjs)
	return po.InvClient.Replace(localInv, localIds)
}

// preventDeleteAnnotation returns true if the "onRemove:keep"
// annotation exists within the annotation map; false otherwise.
func preventDeleteAnnotation(annotations map[string]string) bool {
	for annotation, value := range annotations {
		if annotation == common.OnRemoveAnnotation {
			if value == common.OnRemoveKeep {
				return true
			}
		}
	}
	return false
}

// createPruneEvent is a helper function to package a prune event.
func createPruneEvent(obj runtime.Object, op event.PruneEventOperation) event.Event {
	return event.Event{
		Type: event.PruneType,
		PruneEvent: event.PruneEvent{
			Type:      event.PruneEventResourceUpdate,
			Operation: op,
			Object:    obj,
		},
	}
}

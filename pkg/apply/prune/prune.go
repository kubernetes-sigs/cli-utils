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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

// PruneOptions encapsulates the necessary information to
// implement the prune functionality.
type PruneOptions struct {
	invClient inventory.InventoryClient
	client    dynamic.Interface
	mapper    meta.RESTMapper
	// Stores the UID for each of the currently applied objects.
	// These UID's are written during the apply, and this data
	// structure is shared. IMPORTANT: the apply task must
	// always complete before this prune is run.
	currentUids sets.String
	// The set of retrieved inventory objects (as Infos) selected
	// by the inventory label. This set should also include the
	// current inventory object. Stored here to make testing
	// easier by manually setting the retrieved inventory infos.

	// InventoryFactoryFunc wraps and returns an interface for the
	// object which will load and store the inventory.
	InventoryFactoryFunc func(*resource.Info) inventory.Inventory
}

// NewPruneOptions returns a struct (PruneOptions) encapsulating the necessary
// information to run the prune. Returns an error if an error occurs
// gathering this information.
func NewPruneOptions(currentUids sets.String) *PruneOptions {
	po := &PruneOptions{currentUids: currentUids}
	po.InventoryFactoryFunc = inventory.WrapInventoryObj
	return po
}

func (po *PruneOptions) Initialize(factory util.Factory, invClient inventory.InventoryClient) error {
	var err error
	po.invClient = invClient
	// Client/Builder fields from the Factory.
	po.client, err = factory.DynamicClient()
	if err != nil {
		return err
	}
	po.mapper, err = factory.ToRESTMapper()
	if err != nil {
		return err
	}
	return nil
}

// Options defines a set of parameters that can be used to tune
// the behavior of the pruner.
type Options struct {
	// DryRun defines whether objects should actually be pruned or if
	// we should just print what would happen without actually doing it.
	DryRun bool

	PropagationPolicy metav1.DeletionPropagation
}

// Prune deletes the set of resources which were previously applied
// (retrieved from previous inventory objects) but omitted in
// the current apply. Prune also delete all previous inventory
// objects. Returns an error if there was a problem.
func (po *PruneOptions) Prune(currentObjects []*resource.Info, eventChannel chan<- event.Event, o Options) error {
	currentInventoryObject, found := inventory.FindInventoryObj(currentObjects)
	if !found {
		return fmt.Errorf("current inventory object not found during prune")
	}
	klog.V(7).Infof("prune current inventory object: %s/%s",
		currentInventoryObject.Namespace, currentInventoryObject.Name)

	// Retrieve previous inventory objects, and calculate the
	// union of the previous applies as an inventory set.
	pastObjs, err := po.invClient.GetStoredObjRefs(currentInventoryObject)
	if err != nil {
		return err
	}
	klog.V(4).Infof("prune %d currently applied objects", len(po.currentUids))
	klog.V(4).Infof("prune %d previously applied objects", len(pastObjs))
	// Iterate through set of all previously applied objects.
	for _, past := range pastObjs {
		mapping, err := po.mapper.RESTMapping(past.GroupKind)
		if err != nil {
			return err
		}
		namespacedClient := po.client.Resource(mapping.Resource).Namespace(past.Namespace)
		obj, err := namespacedClient.Get(past.Name, metav1.GetOptions{})
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
		// If this previously applied object is not also a currently applied
		// object, then it has been omitted--prune it. If the previously
		// applied object is part of the current apply set, skip it.
		uid := string(metadata.GetUID())
		klog.V(7).Infof("prune previously applied object UID: %s", uid)
		if po.currentUids.Has(uid) {
			klog.V(7).Infof("prune object in current apply; do not prune: %s", uid)
			continue
		}
		// Handle lifecycle directive preventing deletion.
		if preventDeleteAnnotation(metadata.GetAnnotations()) {
			klog.V(7).Infof("prune object lifecycle directive; do not prune: %s", uid)
			eventChannel <- event.Event{
				Type: event.PruneType,
				PruneEvent: event.PruneEvent{
					Type:      event.PruneEventResourceUpdate,
					Operation: event.PruneSkipped,
					Object:    obj,
				},
			}
			continue
		}
		if !o.DryRun {
			klog.V(7).Infof("prune object delete: %s/%s", past.Namespace, past.Name)
			err = namespacedClient.Delete(past.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
		eventChannel <- event.Event{
			Type: event.PruneType,
			PruneEvent: event.PruneEvent{
				Type:      event.PruneEventResourceUpdate,
				Operation: event.Pruned,
				Object:    obj,
			},
		}
	}
	// Delete previous inventory objects.
	pastInventories, err := po.invClient.GetPreviousInventoryObjects(currentInventoryObject)
	if err != nil {
		return err
	}
	for _, pastGroupInfo := range pastInventories {
		if !o.DryRun {
			klog.V(7).Infof("prune delete previous inventory object: %s/%s",
				pastGroupInfo.Namespace, pastGroupInfo.Name)
			err = po.client.Resource(pastGroupInfo.Mapping.Resource).
				Namespace(pastGroupInfo.Namespace).
				Delete(pastGroupInfo.Name, &metav1.DeleteOptions{
					PropagationPolicy: &o.PropagationPolicy,
				})
			if err != nil {
				return err
			}
		}
		eventChannel <- event.Event{
			Type: event.PruneType,
			PruneEvent: event.PruneEvent{
				Type:      event.PruneEventResourceUpdate,
				Operation: event.Pruned,
				Object:    pastGroupInfo.Object,
			},
		}
	}
	return nil
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

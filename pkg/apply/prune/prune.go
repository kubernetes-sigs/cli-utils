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
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/validation"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// PruneOptions encapsulates the necessary information to
// implement the prune functionality.
type PruneOptions struct {
	client  dynamic.Interface
	builder *resource.Builder
	mapper  meta.RESTMapper
	// The currently applied objects (as Infos), including the
	// current inventory object. These objects are used to
	// calculate the prune set after retrieving the previous
	// inventory objects.
	currentInventoryObject *resource.Info
	// Stores the UID for each of the currently applied objects.
	// These UID's are written during the apply, and this data
	// structure is shared. IMPORTANT: the apply task must
	// always complete before this prune is run.
	currentUids sets.String
	// The set of retrieved inventory objects (as Infos) selected
	// by the inventory label. This set should also include the
	// current inventory object. Stored here to make testing
	// easier by manually setting the retrieved inventory infos.
	pastInventoryObjects      []*resource.Info
	retrievedInventoryObjects bool

	validator validation.Schema

	// TODO: DeleteOptions--cascade?
}

// NewPruneOptions returns a struct (PruneOptions) encapsulating the necessary
// information to run the prune. Returns an error if an error occurs
// gathering this information.
func NewPruneOptions(currentUids sets.String) *PruneOptions {
	po := &PruneOptions{currentUids: currentUids}
	return po
}

func (po *PruneOptions) Initialize(factory util.Factory) error {
	var err error
	// Client/Builder fields from the Factory.
	po.client, err = factory.DynamicClient()
	if err != nil {
		return err
	}
	po.builder = factory.NewBuilder()
	po.mapper, err = factory.ToRESTMapper()
	if err != nil {
		return err
	}
	po.validator, err = factory.Validator(false)
	if err != nil {
		return err
	}
	// Initialize past inventory objects as empty.
	po.pastInventoryObjects = []*resource.Info{}
	po.retrievedInventoryObjects = false
	return nil
}

// getPreviousInventoryObjects returns the set of inventory objects
// that have the same label as the current inventory object. Removes
// the current inventory object from this set. Returns an error
// if there is a problem retrieving the inventory objects.
func (po *PruneOptions) getPreviousInventoryObjects() ([]*resource.Info, error) {
	current, err := infoToObjMetadata(po.currentInventoryObject)
	if err != nil {
		return nil, err
	}
	// Ensures the "pastInventoryObjects" is set.
	if !po.retrievedInventoryObjects {
		if err := po.retrievePreviousInventoryObjects(current.Namespace); err != nil {
			return nil, err
		}
	}
	// Remove the current inventory info from the previous inventory infos.
	pastGroupInfos := []*resource.Info{}
	for _, pastInfo := range po.pastInventoryObjects {
		past, err := infoToObjMetadata(pastInfo)
		if err != nil {
			return nil, err
		}
		if !current.Equals(past) {
			pastGroupInfos = append(pastGroupInfos, pastInfo)
		}
	}
	return pastGroupInfos, nil
}

// retrievePreviousInventoryObjects requests the previous inventory objects
// using the inventory label from the current inventory object. Sets
// the field "pastInventoryObjects". Returns an error if the inventory
// label doesn't exist for the current currentInventoryObject does not
// exist or if the call to retrieve the past inventory objects fails.
func (po *PruneOptions) retrievePreviousInventoryObjects(namespace string) error {
	if po.currentInventoryObject == nil || po.currentInventoryObject.Object == nil {
		return fmt.Errorf("missing current inventory object")
	}
	// Get the inventory label for this inventory object, and create
	// a label selector from it.
	inventoryLabel, err := retrieveInventoryLabel(po.currentInventoryObject.Object)
	if err != nil {
		return err
	}
	labelSelector := fmt.Sprintf("%s=%s", common.InventoryLabel, inventoryLabel)
	retrievedInventoryInfos, err := po.builder.
		Unstructured().
		// TODO: Check if this validator is necessary.
		Schema(po.validator).
		ContinueOnError().
		NamespaceParam(namespace).DefaultNamespace().
		ResourceTypes("configmap").
		LabelSelectorParam(labelSelector).
		Flatten().
		Do().
		Infos()
	if err != nil {
		return err
	}
	po.pastInventoryObjects = retrievedInventoryInfos
	po.retrievedInventoryObjects = true
	return nil
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

// unionPastObjs takes a set of inventory objects (infos), returning the
// union of the objects referenced by these inventory objects.
// Returns an error if any of the passed objects are not inventory
// objects, or if unable to retrieve the referenced objects from any
// inventory object.
func unionPastObjs(infos []*resource.Info) ([]object.ObjMetadata, error) {
	objSet := map[string]object.ObjMetadata{}
	for _, info := range infos {
		objs, err := RetrieveObjsFromInventory([]*resource.Info{info})
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			objSet[(*obj).String()] = *obj // De-duping
		}
	}
	pastObjs := make([]object.ObjMetadata, 0, len(objSet))
	for _, obj := range objSet {
		pastObjs = append(pastObjs, obj)
	}
	return pastObjs, nil
}

// Options defines a set of parameters that can be used to tune
// the behavior of the pruner.
type Options struct {
	// DryRun defines whether objects should actually be pruned or if
	// we should just print what would happen without actually doing it.
	DryRun bool
}

// Prune deletes the set of resources which were previously applied
// (retrieved from previous inventory objects) but omitted in
// the current apply. Prune also delete all previous inventory
// objects. Returns an error if there was a problem.
func (po *PruneOptions) Prune(currentObjects []*resource.Info, eventChannel chan<- event.Event, o Options) error {
	currentInventoryObject, found := FindInventoryObj(currentObjects)
	if !found {
		return fmt.Errorf("current inventory object not found during prune")
	}
	po.currentInventoryObject = currentInventoryObject

	// Retrieve previous inventory objects, and calculate the
	// union of the previous applies as an inventory set.
	pastInventoryInfos, err := po.getPreviousInventoryObjects()
	if err != nil {
		return err
	}
	pastObjs, err := unionPastObjs(pastInventoryInfos)
	if err != nil {
		return err
	}
	// Iterate through set of all previously applied objects.
	for _, past := range pastObjs {
		mapping, err := po.mapper.RESTMapping(past.GroupKind)
		if err != nil {
			return err
		}
		namespacedClient := po.client.Resource(mapping.Resource).Namespace(past.Namespace)
		obj, err := namespacedClient.Get(past.Name, metav1.GetOptions{})
		if err != nil {
			// Object not found -- skip it and move to the next object.
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
		if po.currentUids.Has(uid) {
			continue
		}
		if !o.DryRun {
			err = namespacedClient.Delete(past.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
		eventChannel <- event.Event{
			Type: event.PruneType,
			PruneEvent: event.PruneEvent{
				Type:   event.PruneEventResourceUpdate,
				Object: obj,
			},
		}
	}
	// Delete previous inventory objects.
	for _, pastGroupInfo := range pastInventoryInfos {
		if !o.DryRun {
			err = po.client.Resource(pastGroupInfo.Mapping.Resource).
				Namespace(pastGroupInfo.Namespace).
				Delete(pastGroupInfo.Name, &metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
		eventChannel <- event.Event{
			Type: event.PruneType,
			PruneEvent: event.PruneEvent{
				Type:   event.PruneEventResourceUpdate,
				Object: pastGroupInfo.Object,
			},
		}
	}
	return nil
}

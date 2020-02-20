// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Prune functionality deletes previously applied objects
// which are subsequently omitted in further apply operations.
// This functionality relies on "grouping" objects to store
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
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/validation"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
)

// PruneOptions encapsulates the necessary information to
// implement the prune functionality.
type PruneOptions struct {
	client    dynamic.Interface
	builder   *resource.Builder
	mapper    meta.RESTMapper
	namespace string
	// The currently applied objects (as Infos), including the
	// current grouping object. These objects are used to
	// calculate the prune set after retreiving the previous
	// grouping objects.
	currentGroupingObject *resource.Info
	// The set of retrieved grouping objects (as Infos) selected
	// by the grouping label. This set should also include the
	// current grouping object. Stored here to make testing
	// easier by manually setting the retrieved grouping infos.
	pastGroupingObjects      []*resource.Info
	retrievedGroupingObjects bool

	DryRun    bool
	validator validation.Schema

	// TODO: DeleteOptions--cascade?
}

// NewPruneOptions returns a struct (PruneOptions) encapsulating the necessary
// information to run the prune. Returns an error if an error occurs
// gathering this information.
func NewPruneOptions() *PruneOptions {
	po := &PruneOptions{}
	return po
}

func (po *PruneOptions) Initialize(factory util.Factory, namespace string) error {
	var err error
	// Fields copied from ApplyOptions.
	po.namespace = namespace
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
	return nil
}

// getPreviousGroupingObjects returns the set of grouping objects
// that have the same label as the current grouping object. Removes
// the current grouping object from this set. Returns an error
// if there is a problem retrieving the grouping objects.
func (po *PruneOptions) getPreviousGroupingObjects() ([]*resource.Info, error) {
	// Ensures the "pastGroupingObjects" is set.
	if !po.retrievedGroupingObjects {
		if err := po.retrievePreviousGroupingObjects(); err != nil {
			return nil, err
		}
	}
	// Remove the current grouping info from the previous grouping infos.
	current, err := infoToObjMetadata(po.currentGroupingObject)
	if err != nil {
		return nil, err
	}
	pastGroupInfos := []*resource.Info{}
	for _, pastInfo := range po.pastGroupingObjects {
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

// retrievePreviousGroupingObjects requests the previous grouping objects
// using the grouping label from the current grouping object. Sets
// the field "pastGroupingObjects". Returns an error if the grouping
// label doesn't exist for the current currentGroupingObject does not
// exist or if the call to retrieve the past grouping objects fails.
func (po *PruneOptions) retrievePreviousGroupingObjects() error {
	// Get the grouping label for this grouping object, and create
	// a label selector from it.
	if po.currentGroupingObject == nil || po.currentGroupingObject.Object == nil {
		return fmt.Errorf("missing current grouping object")
	}
	groupingLabel, err := retrieveGroupingLabel(po.currentGroupingObject.Object)
	if err != nil {
		return err
	}
	labelSelector := fmt.Sprintf("%s=%s", GroupingLabel, groupingLabel)
	retrievedGroupingInfos, err := po.builder.
		Unstructured().
		// TODO: Check if this validator is necessary.
		Schema(po.validator).
		ContinueOnError().
		NamespaceParam(po.namespace).DefaultNamespace().
		ResourceTypes("configmap").
		LabelSelectorParam(labelSelector).
		Flatten().
		Do().
		Infos()
	if err != nil {
		return err
	}
	po.pastGroupingObjects = retrievedGroupingInfos
	po.retrievedGroupingObjects = true
	return nil
}

// infoToObjMetadata transforms the object represented by the passed "info"
// into its Inventory representation. Returns error if the passed Info
// is nil, or the Object in the Info is empty.
func infoToObjMetadata(info *resource.Info) (*ObjMetadata, error) {
	if info == nil || info.Object == nil {
		return nil, fmt.Errorf("empty resource.Info can not calculate as inventory")
	}
	obj := info.Object
	gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
	return createObjMetadata(info.Namespace, info.Name, gk)
}

// unionPastInventory takes a set of grouping objects (infos), returning the
// union of the objects referenced by these grouping objects as an
// Inventory. Returns an error if any of the passed objects are not
// grouping objects, or if unable to retrieve the inventory from any
// grouping object.
func unionPastInventory(infos []*resource.Info) (*Inventory, error) {
	inventorySet := NewInventory([]*ObjMetadata{})
	for _, info := range infos {
		inv, err := RetrieveInventoryFromGroupingObj([]*resource.Info{info})
		if err != nil {
			return nil, err
		}
		inventorySet.AddItems(inv)
	}
	return inventorySet, nil
}

// calcPruneSet returns the Inventory representing the objects to
// delete (prune). pastGroupInfos are the set of past applied grouping
// objects, storing the inventory of the objects applied at the same time.
// Calculates the prune set as:
//
//   prune set = (prev1 U prev2 U ... U prevN) - (curr1, curr2, ..., currN)
//
// Returns an error if we are unable to retrieve the set of previously
// applied objects, or if we are unable to get the currently applied objects
// from the current grouping object.
func (po *PruneOptions) calcPruneSet(pastGroupingInfos []*resource.Info) (*Inventory, error) {
	pastInventory, err := unionPastInventory(pastGroupingInfos)
	if err != nil {
		return nil, err
	}
	// Current grouping object as inventory set.
	c := []*resource.Info{po.currentGroupingObject}
	currentInv, err := RetrieveInventoryFromGroupingObj(c)
	if err != nil {
		return nil, err
	}
	return pastInventory.Subtract(NewInventory(currentInv))
}

// Prune deletes the set of resources which were previously applied
// (retrieved from previous grouping objects) but omitted in
// the current apply. Prune also delete all previous grouping
// objects. Returns an error if there was a problem.
func (po *PruneOptions) Prune(currentObjects []*resource.Info, eventChannel chan<- event.Event) error {
	currentGroupingObject, found := FindGroupingObject(currentObjects)
	if !found {
		return fmt.Errorf("current grouping object not found during prune")
	}
	po.currentGroupingObject = currentGroupingObject
	// Initialize past grouping objects as empty.
	po.pastGroupingObjects = []*resource.Info{}
	po.retrievedGroupingObjects = false

	// Retrieve previous grouping objects, and calculate the
	// union of the previous applies as an inventory set.
	pastGroupingInfos, err := po.getPreviousGroupingObjects()
	if err != nil {
		return err
	}
	pruneSet, err := po.calcPruneSet(pastGroupingInfos)
	if err != nil {
		return err
	}
	// Delete the prune objects.
	for _, inv := range pruneSet.GetItems() {
		mapping, err := po.mapper.RESTMapping(inv.GroupKind)
		if err != nil {
			return err
		}
		// Fetching the resource here before deletion seems a bit unnecessary, but
		// it allows us to work with the ResourcePrinter.
		namespacedClient := po.client.Resource(mapping.Resource).Namespace(inv.Namespace)
		obj, err := namespacedClient.Get(inv.Name, metav1.GetOptions{})
		if err != nil {
			// Do not return if object to prune (delete) is not found
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if !po.DryRun {
			err = namespacedClient.Delete(inv.Name, &metav1.DeleteOptions{})
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
	// Delete previous grouping objects.
	for _, pastGroupInfo := range pastGroupingInfos {
		if !po.DryRun {
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

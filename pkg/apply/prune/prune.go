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
	validator                 validation.Schema
	// InventoryFactoryFunc wraps and returns an interface for the
	// object which will load and store the inventory.
	InventoryFactoryFunc func(*resource.Info) Inventory
}

// NewPruneOptions returns a struct (PruneOptions) encapsulating the necessary
// information to run the prune. Returns an error if an error occurs
// gathering this information.
func NewPruneOptions(currentUids sets.String) *PruneOptions {
	po := &PruneOptions{currentUids: currentUids}
	po.InventoryFactoryFunc = WrapInventoryObj
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

// GetPreviousInventoryObjects returns the set of inventory objects
// that have the same label as the current inventory object. Removes
// the current inventory object from this set. Returns an error
// if there is a problem retrieving the inventory objects.
func (po *PruneOptions) GetPreviousInventoryObjects(currentInv *resource.Info) ([]Inventory, error) {
	current, err := infoToObjMetadata(currentInv)
	if err != nil {
		return nil, err
	}
	label, err := retrieveInventoryLabel(currentInv.Object)
	if err != nil {
		return nil, err
	}
	if _, err := po.retrievePreviousInventoryObjects(current, label); err != nil {
		return nil, err
	}
	// Remove the current inventory info from the previous inventory infos.
	pastInventoryInfos := []*resource.Info{}
	pastInventories := []Inventory{}
	for _, pastInfo := range po.pastInventoryObjects {
		past, err := infoToObjMetadata(pastInfo)
		if err != nil {
			return nil, err
		}
		if !current.Equals(past) {
			pastInv := po.InventoryFactoryFunc(pastInfo)
			pastInventories = append(pastInventories, pastInv)
			pastInventoryInfos = append(pastInventoryInfos, pastInfo)
		}
	}
	po.pastInventoryObjects = pastInventoryInfos
	return pastInventories, nil
}

// retrievePreviousInventoryObjects requests the previous inventory objects
// using the inventory label from the current inventory object. Sets
// the field "pastInventoryObjects". Returns an error if the inventory
// label doesn't exist for the current currentInventoryObject does not
// exist or if the call to retrieve the past inventory objects fails.
func (po *PruneOptions) retrievePreviousInventoryObjects(current *object.ObjMetadata, label string) ([]*resource.Info, error) {
	if po.retrievedInventoryObjects {
		return po.pastInventoryObjects, nil
	}
	mapping, err := po.mapper.RESTMapping(current.GroupKind)
	if err != nil {
		return nil, err
	}
	groupResource := mapping.Resource.GroupResource().String()
	namespace := current.Namespace
	labelSelector := fmt.Sprintf("%s=%s", common.InventoryLabel, label)
	klog.V(4).Infof("prune inventory object fetch: %s/%s/%s", groupResource, namespace, labelSelector)
	retrievedInventoryInfos, err := po.builder.
		Unstructured().
		// TODO: Check if this validator is necessary.
		Schema(po.validator).
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
	po.pastInventoryObjects = retrievedInventoryInfos
	po.retrievedInventoryObjects = true
	klog.V(4).Infof("prune %d inventory objects found", len(po.pastInventoryObjects))
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

// UnionPastObjs takes a set of inventory objects (infos), returning the
// union of the objects referenced by these inventory objects.
// Returns an error if any of the passed objects are not inventory
// objects, or if unable to retrieve the referenced objects from any
// inventory object.
func UnionPastObjs(pastInvs []Inventory) ([]object.ObjMetadata, error) {
	objSet := map[string]object.ObjMetadata{}
	for _, inv := range pastInvs {
		objs, err := inv.Load()
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			objSet[obj.String()] = obj // De-duping
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

	PropagationPolicy metav1.DeletionPropagation
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
	klog.V(7).Infof("prune current inventory object: %s/%s",
		currentInventoryObject.Namespace, currentInventoryObject.Name)

	// Retrieve previous inventory objects, and calculate the
	// union of the previous applies as an inventory set.
	pastInventories, err := po.GetPreviousInventoryObjects(currentInventoryObject)
	if err != nil {
		return err
	}
	pastObjs, err := UnionPastObjs(pastInventories)
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
	for _, pastGroupInfo := range po.pastInventoryObjects {
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

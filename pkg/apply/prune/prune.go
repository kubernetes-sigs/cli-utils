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
// but omitted in the current apply. Calculates the set of objects
// to prune by removing the currently applied objects from the union
// set of the previously applied objects and currently applied objects
// stored in the cluster inventory. As a final step, stores the current
// inventory which is all the successfully applied objects and the
// prune failures. Does not stop when encountering prune failures.
// Returns an error for unrecoverable errors.
//
// Parameters:
//   localInv - locally read inventory object
//   localObjs - locally read, currently applied (attempted) objects
//   currentUIDs - UIDs for successfully applied objects
//   taskContext - task for apply/prune
func (po *PruneOptions) Prune(localInv inventory.InventoryInfo,
	localObjs []*unstructured.Unstructured,
	currentUIDs sets.String,
	taskContext *taskrunner.TaskContext,
	o Options) error {
	// Validate parameters
	if localInv == nil {
		return fmt.Errorf("the local inventory object can't be nil")
	}
	// Get the list of Object Meta from the local objects.
	localIds := object.UnstructuredsToObjMetas(localObjs)
	// Create the set of namespaces for currently (locally) applied objects, including
	// the namespace the inventory object lives in (if it's not cluster-scoped). When
	// pruning, check this set of namespaces to ensure these namespaces are not deleted.
	localNamespaces := localNamespaces(localInv, localIds)
	clusterInv, err := po.InvClient.GetClusterObjs(localInv)
	if err != nil {
		return err
	}
	klog.V(4).Infof("prune: %d objects attempted to apply", len(localIds))
	klog.V(4).Infof("prune: %d objects successfully applied", len(currentUIDs))
	klog.V(4).Infof("prune: %d union objects stored in cluster inventory", len(clusterInv))
	pruneObjs := object.SetDiff(clusterInv, localIds)
	klog.V(4).Infof("prune: %d objects to prune (clusterInv - localIds)", len(pruneObjs))
	// Sort the resources in reverse order using the same rules as is
	// used for apply.
	sort.Sort(sort.Reverse(ordering.SortableMetas(pruneObjs)))
	// Store prune failures to ensure they remain in the inventory.
	pruneFailures := []object.ObjMetadata{}
	for _, pruneObj := range pruneObjs {
		klog.V(5).Infof("attempting prune: %s", pruneObj)
		obj, err := po.getObject(pruneObj)
		if err != nil {
			// Object not found in cluster, so no need to delete it; skip to next object.
			if apierrors.IsNotFound(err) {
				klog.V(5).Infof("%s/%s not found in cluster--skipping",
					pruneObj.Namespace, pruneObj.Name)
				continue
			}
			if klog.V(5) {
				klog.Errorf("prune obj (%s/%s) UID retrival error: %s",
					pruneObj.Namespace, pruneObj.Name, err)
			}
			pruneFailures = append(pruneFailures, pruneObj)
			taskContext.EventChannel() <- createPruneFailedEvent(pruneObj, err)
			taskContext.CaptureResourceFailure(pruneObj)
			continue
		}
		// Do not prune objects that are in set of currently applied objects.
		uid := string(obj.GetUID())
		if currentUIDs.Has(uid) {
			klog.V(5).Infof("prune object in current apply; do not prune: %s", uid)
			continue
		}
		// Handle lifecycle directive preventing deletion.
		if !canPrune(localInv, obj, o.InventoryPolicy, uid) {
			klog.V(4).Infof("skip prune for lifecycle directive %s/%s", pruneObj.Namespace, pruneObj.Name)
			taskContext.EventChannel() <- createPruneEvent(pruneObj, obj, event.PruneSkipped)
			pruneFailures = append(pruneFailures, pruneObj)
			continue
		}
		// If regular pruning (not destroying), skip deleting namespace containing
		// currently applied objects.
		if !po.Destroy {
			if pruneObj.GroupKind == object.CoreV1Namespace.GroupKind() &&
				localNamespaces.Has(pruneObj.Name) {
				klog.V(4).Infof("skip pruning namespace: %s", pruneObj.Name)
				taskContext.EventChannel() <- createPruneEvent(pruneObj, obj, event.PruneSkipped)
				pruneFailures = append(pruneFailures, pruneObj)
				taskContext.CaptureResourceFailure(pruneObj)
				continue
			}
		}
		if !o.DryRunStrategy.ClientOrServerDryRun() {
			klog.V(4).Infof("prune object delete: %s/%s", pruneObj.Namespace, pruneObj.Name)
			namespacedClient, err := po.namespacedClient(pruneObj)
			if err != nil {
				if klog.V(4) {
					klog.Errorf("prune failed for %s/%s (%s)", pruneObj.Namespace, pruneObj.Name, err)
				}
				taskContext.EventChannel() <- createPruneFailedEvent(pruneObj, err)
				pruneFailures = append(pruneFailures, pruneObj)
				taskContext.CaptureResourceFailure(pruneObj)
				continue
			}
			err = namespacedClient.Delete(context.TODO(), pruneObj.Name, metav1.DeleteOptions{})
			if err != nil {
				if klog.V(4) {
					klog.Errorf("prune failed for %s/%s (%s)", pruneObj.Namespace, pruneObj.Name, err)
				}
				taskContext.EventChannel() <- createPruneFailedEvent(pruneObj, err)
				pruneFailures = append(pruneFailures, pruneObj)
				taskContext.CaptureResourceFailure(pruneObj)
				continue
			}
		}
		taskContext.EventChannel() <- createPruneEvent(pruneObj, obj, event.Pruned)
	}
	// Final inventory equals applied objects and prune failures.
	appliedResources := taskContext.AppliedResources()
	finalInventory := append(appliedResources, pruneFailures...)
	return po.InvClient.Replace(localInv, finalInventory)
}

func (po *PruneOptions) namespacedClient(obj object.ObjMetadata) (dynamic.ResourceInterface, error) {
	mapping, err := po.mapper.RESTMapping(obj.GroupKind)
	if err != nil {
		return nil, err
	}
	return po.client.Resource(mapping.Resource).Namespace(obj.Namespace), nil
}

func (po *PruneOptions) getObject(obj object.ObjMetadata) (*unstructured.Unstructured, error) {
	namespacedClient, err := po.namespacedClient(obj)
	if err != nil {
		return nil, err
	}
	return namespacedClient.Get(context.TODO(), obj.Name, metav1.GetOptions{})
}

// localNamespaces returns a set of strings of all the namespaces
// for the passed non cluster-scoped localObjs, plus the namespace
// of the passed inventory object.
func localNamespaces(localInv inventory.InventoryInfo, localObjs []object.ObjMetadata) sets.String {
	namespaces := sets.NewString()
	for _, obj := range localObjs {
		namespace := strings.TrimSpace(strings.ToLower(obj.Namespace))
		if namespace != "" {
			namespaces.Insert(namespace)
		}
	}
	invNamespace := strings.TrimSpace(strings.ToLower(localInv.Namespace()))
	if invNamespace != "" {
		namespaces.Insert(invNamespace)
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

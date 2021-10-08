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
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
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
}

// NewPruneOptions returns a struct (PruneOptions) encapsulating the necessary
// information to run the prune. Returns an error if an error occurs
// gathering this information.
func NewPruneOptions(factory util.Factory, invClient inventory.InventoryClient) (*PruneOptions, error) {
	// Client/Builder fields from the Factory.
	client, err := factory.DynamicClient()
	if err != nil {
		return nil, err
	}
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	return &PruneOptions{
		InvClient: invClient,
		Client:    client,
		Mapper:    mapper,
	}, nil
}

// Options defines a set of parameters that can be used to tune
// the behavior of the pruner.
type Options struct {
	// DryRunStrategy defines whether objects should actually be pruned or if
	// we should just print what would happen without actually doing it.
	DryRunStrategy common.DryRunStrategy

	PropagationPolicy metav1.DeletionPropagation

	// True if we are destroying, which deletes the inventory object
	// as well (possibly) the inventory namespace.
	Destroy bool
}

// Prune deletes the set of passed pruneObjs. A prune skip/failure is
// captured in the TaskContext, so we do not lose track of these
// objects from the inventory. The passed prune filters are used to
// determine if permission exists to delete the object. An example
// of a prune filter is PreventDeleteFilter, which checks if an
// annotation exists on the object to ensure the objects is not
// deleted (e.g. a PersistentVolume that we do no want to
// automatically prune/delete).
//
// Parameters:
//   pruneObjs - objects to prune (delete)
//   pruneFilters - list of filters for deletion permission
//   taskContext - task for apply/prune
//   taskName - name of the parent task group, for events
//   o - options for dry-run
func (po *PruneOptions) Prune(pruneObjs object.UnstructuredSet,
	pruneFilters []filter.ValidationFilter,
	taskContext *taskrunner.TaskContext,
	taskName string,
	o Options,
) error {
	eventFactory := CreateEventFactory(o.Destroy, taskName)
	// Iterate through objects to prune (delete). If an object is not pruned
	// and we need to keep it in the inventory, we must capture the prune failure.
	for _, pruneObj := range pruneObjs {
		pruneID := object.UnstructuredToObjMetaOrDie(pruneObj)
		klog.V(5).Infof("attempting prune: %s", pruneID)
		// Check filters to see if we're prevented from pruning/deleting object.
		var filtered bool
		var reason string
		var err error
		for _, filter := range pruneFilters {
			klog.V(6).Infof("prune filter %s: %s", filter.Name(), pruneID)
			filtered, reason, err = filter.Filter(pruneObj)
			if err != nil {
				if klog.V(5).Enabled() {
					klog.Errorf("error during %s, (%s): %s", filter.Name(), pruneID, err)
				}
				taskContext.EventChannel() <- eventFactory.CreateFailedEvent(pruneID, err)
				taskContext.CapturePruneFailure(pruneID)
				break
			}
			if filtered {
				klog.V(4).Infof("prune filtered (filter: %q, resource: %q, reason: %q)", filter.Name(), pruneID, reason)
				taskContext.EventChannel() <- eventFactory.CreateSkippedEvent(pruneObj, reason)
				taskContext.CapturePruneFailure(pruneID)
				break
			}
		}
		if filtered || err != nil {
			continue
		}
		// Filters passed--actually delete object if not dry run.
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
			err = namespacedClient.Delete(context.TODO(), pruneID.Name, metav1.DeleteOptions{
				PropagationPolicy: &o.PropagationPolicy,
			})
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
	localObjs object.UnstructuredSet, o Options) (object.UnstructuredSet, error) {
	localIds := object.UnstructuredsToObjMetasOrDie(localObjs)
	prevInvIds, err := po.InvClient.GetClusterObjs(inv, o.DryRunStrategy)
	if err != nil {
		return nil, err
	}
	pruneIds := prevInvIds.Diff(localIds)
	pruneObjs := object.UnstructuredSet{}
	for _, pruneID := range pruneIds {
		pruneObj, err := po.GetObject(pruneID)
		if err != nil {
			if meta.IsNoMatchError(err) {
				klog.V(4).Infof("skip pruning obj %s/%s: the resource type is unrecognized by the cluster (kind: %s, group %s)",
					pruneID.Namespace, pruneID.Name, pruneID.GroupKind.Kind, pruneID.GroupKind.Group)
				continue
			} else if apierrors.IsNotFound(err) {
				// If prune object is not in cluster, no need to prune it--skip.
				klog.V(4).Infof("skip pruning obj %s/%s: not found in the cluster",
					pruneID.Namespace, pruneID.Name)
				continue
			}
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

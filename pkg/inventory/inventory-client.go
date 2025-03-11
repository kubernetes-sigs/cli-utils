// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Client expresses an interface for interacting with
// objects which store references to objects (inventory objects).
type Client interface {
	// GetClusterObjs returns the set of previously applied objects as ObjMetadata,
	// or an error if one occurred. This set of previously applied object references
	// is stored in the inventory objects living in the cluster.
	GetClusterObjs(ctx context.Context, inv Info) (object.ObjMetadataSet, error)
	// Merge applies the union of the passed objects with the currently
	// stored objects in the inventory object. Returns the set of
	// objects which are not in the passed objects (objects to be pruned).
	// Otherwise, returns an error if one happened.
	Merge(ctx context.Context, inv Info, objs object.ObjMetadataSet, dryRun common.DryRunStrategy) (object.ObjMetadataSet, error)
	// Replace replaces the set of objects stored in the inventory
	// object with the passed set of objects, or an error if one occurs.
	Replace(ctx context.Context, inv Info, objs object.ObjMetadataSet, status []actuation.ObjectStatus, dryRun common.DryRunStrategy) error
	// DeleteInventoryObj deletes the passed inventory object from the APIServer.
	DeleteInventoryObj(ctx context.Context, inv Info, dryRun common.DryRunStrategy) error
	// ListClusterInventoryObjs returns a map mapping from inventory name to a list of cluster inventory objects
	ListClusterInventoryObjs(ctx context.Context) (map[string]object.ObjMetadataSet, error)
}

// ClusterClient is a concrete implementation of the
// Client interface.
type ClusterClient struct {
	dc                    dynamic.Interface
	discoveryClient       discovery.CachedDiscoveryInterface
	mapper                meta.RESTMapper
	InventoryFactoryFunc  StorageFactoryFunc
	invToUnstructuredFunc ToUnstructuredFunc
	statusPolicy          StatusPolicy
	gvk                   schema.GroupVersionKind
}

var _ Client = &ClusterClient{}

// NewClient returns a concrete implementation of the
// Client interface or an error.
func NewClient(factory cmdutil.Factory,
	invFunc StorageFactoryFunc,
	invToUnstructuredFunc ToUnstructuredFunc,
	statusPolicy StatusPolicy,
	gvk schema.GroupVersionKind,
) (*ClusterClient, error) {
	dc, err := factory.DynamicClient()
	if err != nil {
		return nil, err
	}
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	discoveryClinet, err := factory.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	clusterClient := ClusterClient{
		dc:                    dc,
		discoveryClient:       discoveryClinet,
		mapper:                mapper,
		InventoryFactoryFunc:  invFunc,
		invToUnstructuredFunc: invToUnstructuredFunc,
		statusPolicy:          statusPolicy,
		gvk:                   gvk,
	}
	return &clusterClient, nil
}

// Merge stores the union of the passed objects with the objects currently
// stored in the cluster inventory object. Retrieves and caches the cluster
// inventory object. Returns the set differrence of the cluster inventory
// objects and the currently applied objects. This is the set of objects
// to prune. Creates the initial cluster inventory object storing the passed
// objects if an inventory object does not exist. Returns an error if one
// occurred.
func (cic *ClusterClient) Merge(ctx context.Context, localInv Info, objs object.ObjMetadataSet, dryRun common.DryRunStrategy) (object.ObjMetadataSet, error) {
	pruneIDs := object.ObjMetadataSet{}
	invObj, err := cic.invToUnstructuredFunc(localInv)
	if err != nil {
		return pruneIDs, err
	}
	clusterInv, err := cic.getClusterInventoryInfo(ctx, localInv)
	if err != nil {
		return pruneIDs, err
	}

	// Inventory does not exist on the cluster.
	if clusterInv == nil {
		// Wrap inventory object and store the inventory in it.
		var status []actuation.ObjectStatus
		if cic.statusPolicy == StatusPolicyAll {
			status = getObjStatus(nil, objs)
		}
		inv, err := cic.InventoryFactoryFunc(invObj)
		if err != nil {
			return nil, err
		}
		if err := inv.Store(objs, status); err != nil {
			return nil, err
		}
		klog.V(4).Infof("creating initial inventory object with %d objects", len(objs))

		if dryRun.ClientOrServerDryRun() {
			klog.V(4).Infof("dry-run create inventory object: not created")
			return nil, nil
		}

		err = inv.Apply(ctx, cic.dc, cic.mapper, cic.statusPolicy)
		return nil, err
	}

	// Update existing cluster inventory with merged union of objects
	clusterObjs, err := cic.GetClusterObjs(ctx, localInv)
	if err != nil {
		return pruneIDs, err
	}
	pruneIDs = clusterObjs.Diff(objs)
	unionObjs := clusterObjs.Union(objs)
	var status []actuation.ObjectStatus
	if cic.statusPolicy == StatusPolicyAll {
		status = getObjStatus(pruneIDs, unionObjs)
	}
	klog.V(4).Infof("num objects to prune: %d", len(pruneIDs))
	klog.V(4).Infof("num merged objects to store in inventory: %d", len(unionObjs))
	wrappedInv, err := cic.InventoryFactoryFunc(clusterInv)
	if err != nil {
		return pruneIDs, err
	}
	if err = wrappedInv.Store(unionObjs, status); err != nil {
		return pruneIDs, err
	}

	// Update not required when all objects in inventory are the same and
	// status does not need to be updated. If status is stored, always update the
	// inventory to store the latest status.
	if objs.Equal(clusterObjs) && cic.statusPolicy == StatusPolicyNone {
		return pruneIDs, nil
	}

	if dryRun.ClientOrServerDryRun() {
		klog.V(4).Infof("dry-run create inventory object: not created")
		return pruneIDs, nil
	}
	err = wrappedInv.Apply(ctx, cic.dc, cic.mapper, cic.statusPolicy)
	return pruneIDs, err
}

// Replace stores the passed objects in the cluster inventory object, or
// an error if one occurred.
func (cic *ClusterClient) Replace(ctx context.Context, localInv Info, objs object.ObjMetadataSet, status []actuation.ObjectStatus,
	dryRun common.DryRunStrategy) error {
	// Skip entire function for dry-run.
	if dryRun.ClientOrServerDryRun() {
		klog.V(4).Infoln("dry-run replace inventory object: not applied")
		return nil
	}
	clusterInv, err := cic.getClusterInventoryInfo(ctx, localInv)
	if err != nil {
		return fmt.Errorf("failed to read inventory from cluster: %w", err)
	}

	clusterObjs, err := cic.GetClusterObjs(ctx, localInv)
	if err != nil {
		return fmt.Errorf("failed to read inventory objects from cluster: %w", err)
	}

	clusterInv, wrappedInv, err := cic.replaceInventory(clusterInv, objs, status)
	if err != nil {
		return err
	}

	// Update not required when all objects in inventory are the same and
	// status does not need to be updated. If status is stored, always update the
	// inventory to store the latest status.
	if objs.Equal(clusterObjs) && cic.statusPolicy == StatusPolicyNone {
		return nil
	}

	klog.V(4).Infof("replace cluster inventory: %s/%s", clusterInv.GetNamespace(), clusterInv.GetName())
	klog.V(4).Infof("replace cluster inventory %d objects", len(objs))

	if err := wrappedInv.ApplyWithPrune(ctx, cic.dc, cic.mapper, cic.statusPolicy, objs); err != nil {
		return fmt.Errorf("failed to write updated inventory to cluster: %w", err)
	}

	return nil
}

// replaceInventory stores the passed objects into the passed inventory object.
func (cic *ClusterClient) replaceInventory(inv *unstructured.Unstructured, objs object.ObjMetadataSet,
	status []actuation.ObjectStatus) (*unstructured.Unstructured, Storage, error) {
	if cic.statusPolicy == StatusPolicyNone {
		status = nil
	}
	wrappedInv, err := cic.InventoryFactoryFunc(inv)
	if err != nil {
		return nil, nil, err
	}
	if err := wrappedInv.Store(objs, status); err != nil {
		return nil, nil, err
	}
	clusterInv, err := wrappedInv.GetObject()
	if err != nil {
		return nil, nil, err
	}

	return clusterInv, wrappedInv, nil
}

// DeleteInventoryObj deletes the inventory object from the cluster.
func (cic *ClusterClient) DeleteInventoryObj(ctx context.Context, localInv Info, dryRun common.DryRunStrategy) error {
	if localInv == nil {
		return fmt.Errorf("retrieving cluster inventory object with nil local inventory")
	}
	switch localInv.Strategy() {
	case NameStrategy:
		obj, err := cic.invToUnstructuredFunc(localInv)
		if err != nil {
			return err
		}
		return cic.deleteInventoryObjByName(ctx, obj, dryRun)
	case LabelStrategy:
		return cic.deleteInventoryObjsByLabel(ctx, localInv, dryRun)
	default:
		panic(fmt.Errorf("unknown inventory strategy: %s", localInv.Strategy()))
	}
}

func (cic *ClusterClient) deleteInventoryObjsByLabel(ctx context.Context, inv Info, dryRun common.DryRunStrategy) error {
	clusterInvObjs, err := cic.getClusterInventoryObjsByLabel(ctx, inv)
	if err != nil {
		return err
	}
	for _, invObj := range clusterInvObjs {
		if err := cic.deleteInventoryObjByName(ctx, invObj, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// GetClusterObjs returns the objects stored in the cluster inventory object, or
// an error if one occurred.
func (cic *ClusterClient) GetClusterObjs(ctx context.Context, localInv Info) (object.ObjMetadataSet, error) {
	var objs object.ObjMetadataSet
	clusterInv, err := cic.getClusterInventoryInfo(ctx, localInv)
	if err != nil {
		return objs, fmt.Errorf("failed to read inventory from cluster: %w", err)
	}
	// First time; no inventory obj yet.
	if clusterInv == nil {
		return objs, nil
	}
	wrapped, err := cic.InventoryFactoryFunc(clusterInv)
	if err != nil {
		return objs, err
	}
	return wrapped.Load()
}

// getClusterInventoryObj returns a pointer to the cluster inventory object, or
// an error if one occurred. Returns the cached cluster inventory object if it
// has been previously retrieved. Uses the ResourceBuilder to retrieve the
// inventory object in the cluster, using the namespace, group resource, and
// inventory label. Merges multiple inventory objects into one if it retrieves
// more than one (this should be very rare).
//
// TODO(seans3): Remove the special case code to merge multiple cluster inventory
// objects once we've determined that this case is no longer possible.
func (cic *ClusterClient) getClusterInventoryInfo(ctx context.Context, inv Info) (*unstructured.Unstructured, error) {
	clusterInvObjects, err := cic.getClusterInventoryObjs(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("failed to read inventory objects from cluster: %w", err)
	}

	var clusterInv *unstructured.Unstructured
	if len(clusterInvObjects) == 1 {
		clusterInv = clusterInvObjects[0]
	} else if l := len(clusterInvObjects); l > 1 {
		return nil, fmt.Errorf("found %d inventory objects with inventory id %s", l, inv.ID())
	}
	return clusterInv, nil
}

func (cic *ClusterClient) getClusterInventoryObjsByLabel(ctx context.Context, inv Info) (object.UnstructuredSet, error) {
	localInv, err := cic.invToUnstructuredFunc(inv)
	if err != nil {
		return nil, err
	}
	if localInv == nil {
		return nil, fmt.Errorf("retrieving cluster inventory object with nil local inventory")
	}
	localObj := object.UnstructuredToObjMetadata(localInv)
	mapping, err := cic.getMapping(localInv)
	if err != nil {
		return nil, err
	}
	groupResource := mapping.Resource.GroupResource().String()
	namespace := localObj.Namespace
	label, err := retrieveInventoryLabel(localInv)
	if err != nil {
		return nil, err
	}
	labelSelector := fmt.Sprintf("%s=%s", common.InventoryLabel, label)
	klog.V(4).Infof("inventory object fetch by label (group: %q, namespace: %q, selector: %q)", groupResource, namespace, labelSelector)

	uList, err := cic.dc.Resource(mapping.Resource).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}
	var invList []*unstructured.Unstructured
	for i := range uList.Items {
		invList = append(invList, &uList.Items[i])
	}
	return invList, nil
}

func (cic *ClusterClient) getClusterInventoryObjsByName(ctx context.Context, inv Info) (object.UnstructuredSet, error) {
	localInv, err := cic.invToUnstructuredFunc(inv)
	if err != nil {
		return nil, err
	}
	if localInv == nil {
		return nil, fmt.Errorf("retrieving cluster inventory object with nil local inventory")
	}

	mapping, err := cic.getMapping(localInv)
	if err != nil {
		return nil, err
	}

	klog.V(4).Infof("inventory object fetch by name (namespace: %q, name: %q)", inv.Namespace(), inv.Name())
	clusterInv, err := cic.dc.Resource(mapping.Resource).Namespace(inv.Namespace()).
		Get(ctx, inv.Name(), metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) {
		return object.UnstructuredSet{}, nil
	}
	if inv.ID() != "" {
		if inventoryID, err := retrieveInventoryLabel(clusterInv); err != nil {
			return nil, err
		} else if inv.ID() != inventoryID {
			return nil, fmt.Errorf("inventory-id of inventory object %s/%s in cluster doesn't match provided id %q",
				inv.Namespace(), inv.Name(), inv.ID())
		}
	}
	return object.UnstructuredSet{clusterInv}, nil
}

func (cic *ClusterClient) getClusterInventoryObjs(ctx context.Context, inv Info) (object.UnstructuredSet, error) {
	if inv == nil {
		return nil, fmt.Errorf("inventoryInfo must be specified")
	}

	var clusterInvObjects object.UnstructuredSet
	var err error
	switch inv.Strategy() {
	case NameStrategy:
		clusterInvObjects, err = cic.getClusterInventoryObjsByName(ctx, inv)
	case LabelStrategy:
		clusterInvObjects, err = cic.getClusterInventoryObjsByLabel(ctx, inv)
	default:
		panic(fmt.Errorf("unknown inventory strategy: %s", inv.Strategy()))
	}
	return clusterInvObjects, err
}

func (cic *ClusterClient) ListClusterInventoryObjs(ctx context.Context) (map[string]object.ObjMetadataSet, error) {
	// Define the mapping
	mapping, err := cic.mapper.RESTMapping(cic.gvk.GroupKind(), cic.gvk.Version)
	if err != nil {
		return nil, err
	}

	// retrieve the list from the cluster
	clusterInvs, err := cic.dc.Resource(mapping.Resource).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) {
		return map[string]object.ObjMetadataSet{}, nil
	}

	identifiers := make(map[string]object.ObjMetadataSet)

	for i, inv := range clusterInvs.Items {
		invName := inv.GetName()
		identifiers[invName] = object.ObjMetadataSet{}
		invObj, err := cic.InventoryFactoryFunc(&clusterInvs.Items[i])
		if err != nil {
			return nil, err
		}
		wrappedInvObjSlice, err := invObj.Load()
		if err != nil {
			return nil, err
		}
		identifiers[invName] = append(identifiers[invName], wrappedInvObjSlice...)
	}

	return identifiers, nil
}

// deleteInventoryObjByName deletes the passed inventory object from the APIServer, or
// an error if one occurs.
func (cic *ClusterClient) deleteInventoryObjByName(ctx context.Context, obj *unstructured.Unstructured, dryRun common.DryRunStrategy) error {
	if obj == nil {
		return fmt.Errorf("attempting delete a nil inventory object")
	}
	if dryRun.ClientOrServerDryRun() {
		klog.V(4).Infof("dry-run delete inventory object: not deleted")
		return nil
	}

	mapping, err := cic.getMapping(obj)
	if err != nil {
		return err
	}

	klog.V(4).Infof("deleting inventory object: %s/%s", obj.GetNamespace(), obj.GetName())
	return cic.dc.Resource(mapping.Resource).Namespace(obj.GetNamespace()).
		Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
}

// getMapping returns the RESTMapping for the provided resource.
func (cic *ClusterClient) getMapping(obj *unstructured.Unstructured) (*meta.RESTMapping, error) {
	return cic.mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
}

// getObjStatus returns the list of object status
// at the beginning of an apply process.
func getObjStatus(pruneIDs, unionIDs []object.ObjMetadata) []actuation.ObjectStatus {
	status := []actuation.ObjectStatus{}
	for _, obj := range unionIDs {
		status = append(status,
			actuation.ObjectStatus{
				ObjectReference: ObjectReferenceFromObjMetadata(obj),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationPending,
				Reconcile:       actuation.ReconcilePending,
			})
	}
	for _, obj := range pruneIDs {
		status = append(status,
			actuation.ObjectStatus{
				ObjectReference: ObjectReferenceFromObjMetadata(obj),
				Strategy:        actuation.ActuationStrategyDelete,
				Actuation:       actuation.ActuationPending,
				Reconcile:       actuation.ReconcilePending,
			})
	}
	return status
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"fmt"
	"maps"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client expresses an interface for interacting with
// objects which store references to objects (inventory objects).
type Client interface {
	ReadClient
	WriteClient
	Factory
}

// ID is a unique identifier for an inventory object.
// It's used by the inventory client to get/update/delete the object.
// For example, it could be a name or a label depending on the inventory client
// implementation.
type ID string

// String implements the fmt.Stringer interface for ID
func (id ID) String() string {
	return string(id)
}

// Info provides the minimal information for the applier
// to create, look up and update an inventory.
// The inventory object can be any type, the Provider in the applier
// needs to know how to create, look up and update it based
// on the Info.
type Info interface {
	// ID of the inventory object.
	// The inventory client uses this to determine how to get/update the object(s)
	// from the cluster.
	ID() ID

	// Namespace of the inventory object.
	// It should be the value of the field .metadata.namespace.
	Namespace() string

	GetLabels() map[string]string

	GetAnnotations() map[string]string

	DeepCopy() Info
}

type SimpleInfo struct {
	id          ID
	namespace   string
	labels      map[string]string
	annotations map[string]string
}

func (i SimpleInfo) ID() ID {
	return i.id
}

func (i SimpleInfo) Namespace() string {
	return i.namespace
}

func (i SimpleInfo) GetLabels() map[string]string {
	return i.labels
}

func (i SimpleInfo) GetAnnotations() map[string]string {
	return i.annotations
}

func (i SimpleInfo) DeepCopy() Info {
	return SimpleInfo{
		id:          i.id,
		namespace:   i.namespace,
		labels:      maps.Clone(i.labels),
		annotations: maps.Clone(i.annotations),
	}
}

func NewSimpleInfo(id ID, namespace string, labels, annotations map[string]string) SimpleInfo {
	return SimpleInfo{
		id:          id,
		namespace:   namespace,
		labels:      labels,
		annotations: annotations,
	}
}

type SingleObjectInfo struct {
	SimpleInfo

	name string
}

func (i SingleObjectInfo) Name() string {
	return i.name
}

func NewSingleObjectInfo(id ID, nn types.NamespacedName, labels, annotations map[string]string) SingleObjectInfo {
	return SingleObjectInfo{
		SimpleInfo: SimpleInfo{
			id:          id,
			namespace:   nn.Namespace,
			labels:      labels,
			annotations: annotations,
		},
		name: nn.Name,
	}
}

type Factory interface {
	// NewInventory returns an empty initialized inventory object.
	// This is used in the case that there is no existing object on the cluster.
	NewInventory(Info) (Inventory, error)
}

type Inventory interface {
	Info() Info
	ObjectRefs() object.ObjMetadataSet
	ObjectStatuses() []actuation.ObjectStatus
	SetObjectRefs(object.ObjMetadataSet)
	SetObjectStatuses([]actuation.ObjectStatus)
}

type ReadClient interface {
	Get(ctx context.Context, inv Info, opts GetOptions) (Inventory, error)
	List(ctx context.Context, opts ListOptions) ([]Inventory, error)
}

type WriteClient interface {
	CreateOrUpdate(ctx context.Context, inv Inventory, opts UpdateOptions) error
	UpdateStatus(ctx context.Context, inv Inventory, opts UpdateOptions) error
	Delete(ctx context.Context, inv Info, opts DeleteOptions) error
}

type UpdateOptions struct{}

type GetOptions struct{}

type ListOptions struct{}

type DeleteOptions struct{}

var _ Client = &UnstructuredClient{}

// UnstructuredInventory implements Inventory while also tracking the actual
// KRM object from the cluster. This enables the client to update the
// same object and utilize resourceVersion checks.
type UnstructuredInventory struct {
	BaseInventory
	// ClusterObj represents the KRM which was last fetched from the cluster.
	// used by the client implementation to performs updates on the object.
	ClusterObj *unstructured.Unstructured
}

func (ui *UnstructuredInventory) Info() Info {
	if ui.ClusterObj == nil {
		return SingleObjectInfo{}
	}
	// TODO: DeepCopy labels & annotations?
	return NewSingleObjectInfo(ui.ID(), client.ObjectKeyFromObject(ui.ClusterObj),
		maps.Clone(ui.ClusterObj.GetLabels()), maps.Clone(ui.ClusterObj.GetAnnotations()))
}

func (ui *UnstructuredInventory) ID() ID {
	if ui.ClusterObj == nil {
		return ""
	}
	// Empty string if not set.
	return ID(ui.ClusterObj.GetLabels()[common.InventoryLabel])
}

var _ Inventory = &UnstructuredInventory{}

// BaseInventory is a boilerplate struct that contains the basic methods
// to implement Inventory. Can be extended for different inventory implementations.
type BaseInventory struct {
	// Objs and ObjStatuses are in memory representations of the inventory which are
	// read and manipulated by the applier.
	Objs        object.ObjMetadataSet
	ObjStatuses []actuation.ObjectStatus
}

func (inv *BaseInventory) ObjectRefs() object.ObjMetadataSet {
	return inv.Objs
}

func (inv *BaseInventory) ObjectStatuses() []actuation.ObjectStatus {
	return inv.ObjStatuses
}

func (inv *BaseInventory) SetObjectRefs(objs object.ObjMetadataSet) {
	inv.Objs = objs
}

func (inv *BaseInventory) SetObjectStatuses(statuses []actuation.ObjectStatus) {
	inv.ObjStatuses = statuses
}

type FromUnstructuredFunc func(*unstructured.Unstructured) (*UnstructuredInventory, error)
type ToUnstructuredFunc func(*UnstructuredInventory) (*unstructured.Unstructured, error)

// UnstructuredClient implements the inventory client interface for a single unstructured object
type UnstructuredClient struct {
	client           dynamic.NamespaceableResourceInterface
	fromUnstructured FromUnstructuredFunc
	toUnstructured   ToUnstructuredFunc
	gvk              schema.GroupVersionKind
}

func NewUnstructuredClient(factory cmdutil.Factory,
	from FromUnstructuredFunc,
	to ToUnstructuredFunc,
	gvk schema.GroupVersionKind) (*UnstructuredClient, error) {
	dc, err := factory.DynamicClient()
	if err != nil {
		return nil, err
	}
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	unstructuredClient := &UnstructuredClient{
		client:           dc.Resource(mapping.Resource),
		fromUnstructured: from,
		toUnstructured:   to,
		gvk:              gvk,
	}
	return unstructuredClient, nil
}

func (cic *UnstructuredClient) NewInventory(inv Info) (Inventory, error) {
	soi := inv.(SingleObjectInfo)
	obj := &unstructured.Unstructured{}
	obj.SetName(soi.Name())
	obj.SetNamespace(soi.Namespace())
	obj.SetLabels(maps.Clone(soi.GetLabels()))
	obj.SetAnnotations(maps.Clone(soi.GetAnnotations()))
	return cic.fromUnstructured(obj)
}

// Get the in-cluster inventory
func (cic *UnstructuredClient) Get(ctx context.Context, invInfo Info, _ GetOptions) (Inventory, error) {
	inv, ok := invInfo.(SingleObjectInfo)
	if !ok {
		return nil, fmt.Errorf("expected SingleObjectInfo")
	}
	obj, err := cic.client.Namespace(invInfo.Namespace()).Get(ctx, inv.Name(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cic.fromUnstructured(obj)
}

// List the in-cluster inventory
// Used by the CLI commands
func (cic *UnstructuredClient) List(ctx context.Context, _ ListOptions) ([]Inventory, error) {
	objs, err := cic.client.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var inventories []Inventory
	for _, obj := range objs.Items {
		uInv, err := cic.fromUnstructured(&obj)
		if err != nil {
			return nil, err
		}
		inventories = append(inventories, uInv)
	}
	return inventories, nil
}

// CreateOrUpdate the in-cluster inventory
// Updates the unstructured object, or creates it if it doesn't exist
func (cic *UnstructuredClient) CreateOrUpdate(ctx context.Context, inv Inventory, _ UpdateOptions) error {
	ui, ok := inv.(*UnstructuredInventory)
	if !ok {
		return fmt.Errorf("expected UnstructuredInventory")
	}
	if ui == nil {
		return fmt.Errorf("inventory is nil")
	}
	// TODO: avoid deepcopy-ing the labels and annotations
	invInfo := inv.Info().(SingleObjectInfo)
	// Attempt to retry on a resource conflict error to avoid needing to retry the
	// entire Apply/Destroy when there's a transient conflict.
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		create := false
		obj, err := cic.client.Namespace(invInfo.Namespace()).Get(ctx, invInfo.Name(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			create = true
		} else if err != nil {
			return err
		}
		if obj != nil { // Obj is nil when IsNotFound, in this case keep initial/empty obj
			ui.ClusterObj = obj
		}
		uObj, err := cic.toUnstructured(ui)
		if err != nil {
			return err
		}
		var newObj *unstructured.Unstructured
		if create {
			klog.V(4).Infof("creating inventory object %s/%s/%s", cic.gvk, uObj.GetNamespace(), uObj.GetName())
			newObj, err = cic.client.Namespace(uObj.GetNamespace()).Create(ctx, uObj, metav1.CreateOptions{})
			if err != nil {
				return err
			}
		} else {
			klog.V(4).Infof("updating inventory object %s/%s/%s", cic.gvk, uObj.GetNamespace(), uObj.GetName())
			newObj, err = cic.client.Namespace(uObj.GetNamespace()).Update(ctx, uObj, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
		ui.ClusterObj = newObj
		return nil
	})
}

// UpdateStatus updates the status of the in-cluster inventory
// Performs a simple in-place update on the unstructured object
func (cic *UnstructuredClient) UpdateStatus(ctx context.Context, inv Inventory, _ UpdateOptions) error {
	ui, ok := inv.(*UnstructuredInventory)
	if !ok {
		return fmt.Errorf("expected UnstructuredInventory")
	}
	if ui == nil {
		return fmt.Errorf("inventory is nil")
	}
	// TODO: avoid deepcopy-ing the labels and annotations
	invInfo := inv.Info().(SingleObjectInfo)
	// Attempt to retry on a resource conflict error to avoid needing to retry the
	// entire Apply/Destroy when there's a transient conflict.
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj, err := cic.client.Namespace(invInfo.Namespace()).Get(ctx, invInfo.Name(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		ui.ClusterObj = obj
		uObj, err := cic.toUnstructured(ui)
		if err != nil {
			return err
		}
		// Update observedGeneration, if it exists
		_, ok, err = unstructured.NestedInt64(uObj.Object, "status", "observedGeneration")
		if err != nil {
			return err
		}
		if ok {
			err = unstructured.SetNestedField(uObj.Object, uObj.GetGeneration(), "status", "observedGeneration")
			if err != nil {
				return err
			}
		}
		klog.V(4).Infof("updating status of inventory object %s/%s/%s", cic.gvk, uObj.GetNamespace(), uObj.GetName())
		newObj, err := cic.client.Namespace(uObj.GetNamespace()).UpdateStatus(ctx, uObj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updateStatus: %w", err)
		}
		ui.ClusterObj = newObj
		return nil
	})
}

// Delete the in-cluster inventory
// Performs a simple deletion of the unstructured object
func (cic *UnstructuredClient) Delete(ctx context.Context, invInfo Info, _ DeleteOptions) error {
	soi, ok := invInfo.(SingleObjectInfo)
	if !ok {
		return fmt.Errorf("expected SingleObjectInfo")
	}
	err := cic.client.Namespace(soi.Namespace()).Delete(ctx, soi.Name(), metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

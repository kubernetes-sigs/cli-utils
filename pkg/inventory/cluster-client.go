// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Introduces the InventoryConfigMap struct which implements
// the Inventory interface. The InventoryConfigMap wraps a
// ConfigMap resource which stores the set of inventory
// (object metadata).

package inventory

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ClusterClient wraps a Converter and implements the Client interface.
type ClusterClient struct {
	DynamicClient dynamic.Interface
	Mapper        meta.RESTMapper
	Converter     Converter
}

var _ Client = &ClusterClient{}

func (cc *ClusterClient) GroupVersionKind() schema.GroupVersionKind {
	return cc.Converter.GroupVersionKind()
}

// Load the inventory from a ConfigMap.
func (cc *ClusterClient) Load(ctx context.Context, invInfo InventoryInfo) (*actuation.Inventory, error) {
	infoGK := GroupKindFromObjectReference(invInfo.ObjectReference)
	gvk := cc.GroupVersionKind()
	gk := gvk.GroupKind()
	if infoGK != gk {
		return nil, InvalidInventoryTypeError{
			Required: gk,
			Received: infoGK,
		}
	}

	// Request exactly the version supported by the Converter.
	mapping, err := cc.Mapper.RESTMapping(gk, gvk.Version)
	if err != nil {
		return nil, err
	}

	id := ObjMetadataFromObjectReference(invInfo.ObjectReference)
	obj, err := cc.getObject(ctx, id, mapping)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	klog.V(7).Infof("Existing inventory:\n%s", object.YamlStringer{O: obj})

	inv, err := cc.Converter.To(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert inventory: %w", err)
	}

	// Valdate inventory-id after converting, in case it's stored differently.
	if invInfo.ID != InventoryLabel(inv) {
		return nil, fmt.Errorf("inventory-id of inventory object in cluster doesn't match provided id %q", invInfo.ID)
	}

	return inv, nil
}

// Store the inventory as a ConfigMap.
func (cc *ClusterClient) Store(ctx context.Context, inv *actuation.Inventory, dryRun common.DryRunStrategy) error {
	invGK := inv.GroupVersionKind().GroupKind()
	gvk := cc.GroupVersionKind()
	gk := gvk.GroupKind()
	if invGK != gk {
		return InvalidInventoryTypeError{
			Required: gk,
			Received: invGK,
		}
	}

	obj, err := cc.Converter.From(inv)
	if err != nil {
		return fmt.Errorf("failed to convert inventory: %w", err)
	}

	// Request exactly the version supported by the Converter.
	mapping, err := cc.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}
	id := object.UnstructuredToObjMetadata(obj)
	// TODO: use kubectl code to get SSA and CSA impl
	oldObj, err := cc.getObject(ctx, id, mapping)
	if err != nil {
		return err
	}

	var out *unstructured.Unstructured
	if oldObj != nil {
		klog.V(7).Infof("Existing inventory:\n%s", object.YamlStringer{O: oldObj})
		// copy metadata from existing object
		obj.SetUID(oldObj.GetUID())
		obj.SetResourceVersion(oldObj.GetResourceVersion())
		// TODO: should any other metadata be copied/merged?
		out, err = cc.updateObject(ctx, obj, mapping, dryRun)
		if err != nil {
			return err
		}
	} else {
		out, err = cc.createObject(ctx, obj, mapping, dryRun)
		if err != nil {
			return err
		}
	}
	klog.V(7).Infof("Updated inventory:\n%s", object.YamlStringer{O: out})
	return nil
}

func (cc *ClusterClient) Delete(ctx context.Context, invInfo InventoryInfo, dryRun common.DryRunStrategy) error {
	infoGK := GroupKindFromObjectReference(invInfo.ObjectReference)
	gvk := cc.GroupVersionKind()
	gk := gvk.GroupKind()
	if infoGK != gk {
		return InvalidInventoryTypeError{
			Required: gk,
			Received: infoGK,
		}
	}

	// Request exactly the version supported by the Converter.
	mapping, err := cc.Mapper.RESTMapping(gk, gvk.Version)
	if err != nil {
		return err
	}

	id := ObjMetadataFromObjectReference(invInfo.ObjectReference)
	err = cc.deleteObject(ctx, id, mapping, dryRun)
	if err != nil {
		return err
	}
	return nil
}

func (cc *ClusterClient) getObject(ctx context.Context, id object.ObjMetadata, mapping *meta.RESTMapping) (*unstructured.Unstructured, error) {
	klog.V(4).Infof("getting object from cluster: %v", id)
	obj, err := cc.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Get(ctx, id.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get object %q: %w", id, err)
	}
	return obj, nil
}

func (cc *ClusterClient) createObject(ctx context.Context, obj *unstructured.Unstructured, mapping *meta.RESTMapping, dryRun common.DryRunStrategy) (*unstructured.Unstructured, error) {
	id := object.UnstructuredToObjMetadata(obj)
	if dryRun.ClientOrServerDryRun() {
		// TODO: impl server-side dry-run
		klog.V(4).Infoln("creating object in cluster (dry-run): %v", id)
		return nil, nil
	}
	klog.V(4).Infof("creating object in cluster: %v", id)
	out, err := cc.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return out, fmt.Errorf("failed to create object %q: %w", id, err)
	}
	return out, nil
}

func (cc *ClusterClient) updateObject(ctx context.Context, obj *unstructured.Unstructured, mapping *meta.RESTMapping, dryRun common.DryRunStrategy) (*unstructured.Unstructured, error) {
	id := object.UnstructuredToObjMetadata(obj)
	if dryRun.ClientOrServerDryRun() {
		// TODO: impl server-side dry-run
		klog.V(4).Infoln("updating object in cluster (dry-run): %v", id)
		return nil, nil
	}
	klog.V(4).Infof("updating object in cluster: %v", id)

	out, err := cc.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return out, fmt.Errorf("failed to update object %q: %w", id, err)
	}
	return out, nil
}

func (cc *ClusterClient) deleteObject(ctx context.Context, id object.ObjMetadata, mapping *meta.RESTMapping, dryRun common.DryRunStrategy) error {
	if dryRun.ClientOrServerDryRun() {
		// TODO: impl server-side dry-run
		klog.V(4).Infoln("deleting object in cluster (dry-run): %v", id)
		return nil
	}
	klog.V(4).Infof("deleting object in cluster: %v", id)

	err := cc.DynamicClient.Resource(mapping.Resource).
		Namespace(id.Namespace).
		Delete(ctx, id.Name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete object %q: %w", id, err)
	}
	return err
}

type InvalidInventoryTypeError struct {
	Received schema.GroupKind
	Required schema.GroupKind
}

func (iit InvalidInventoryTypeError) Error() string {
	return fmt.Sprintf("invalid inventory type (required: %#v, received: %#v)",
		iit.Required, iit.Received)
}

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type ObjectManager struct {
	Mapper        meta.RESTMapper
	DynamicClient dynamic.Interface
}

func (om *ObjectManager) GetSpecObjects(ctx context.Context, inv *actuation.Inventory) (object.UnstructuredSet, error) {
	invInfo := InventoryInfoFromObject(inv)
	klog.V(4).Infof("getting inventory objects from cluster: %v",
		NewInfoStringer(invInfo))

	// Get the inventory objects from the cluster
	var invObjs object.UnstructuredSet
	for _, ref := range inv.Spec.Objects {
		id := ObjMetadataFromObjectReference(ref)
		obj, err := om.getObject(ctx, id)
		if err != nil {
			if apierrors.IsNotFound(err) {
				klog.V(4).Infof("skip pruning: resource not found: %v",
					NewObjectReferenceStringer(ref))
				continue
			}
			if meta.IsNoMatchError(err) {
				klog.V(4).Infof("skip pruning: resource type not registered: %v",
					NewObjectReferenceStringer(ref))
				continue
			}
			return nil, fmt.Errorf("failed to get object from cluster: %v",
				NewObjectReferenceStringer(ref))
		}
		invObjs = append(invObjs, obj)
	}
	return invObjs, nil
}

func (om *ObjectManager) getObject(ctx context.Context, id object.ObjMetadata) (*unstructured.Unstructured, error) {
	klog.V(4).Infof("getting object from cluster: %v", id)
	mapping, err := om.Mapper.RESTMapping(id.GroupKind)
	if err != nil {
		return nil, err
	}

	var obj *unstructured.Unstructured
	switch mapping.Scope {
	case meta.RESTScopeNamespace:
		obj, err = om.DynamicClient.Resource(mapping.Resource).
			Namespace(id.Namespace).
			Get(ctx, id.Name, metav1.GetOptions{})
	case meta.RESTScopeRoot:
		obj, err = om.DynamicClient.Resource(mapping.Resource).
			Get(ctx, id.Name, metav1.GetOptions{})
	default:
		return nil, fmt.Errorf("invalid scope %q for object: %v", mapping.Scope, id)
	}
	return obj, err
}

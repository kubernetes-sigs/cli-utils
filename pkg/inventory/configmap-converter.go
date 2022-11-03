// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"encoding/json"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ConfigMapConverter implements a converter to serialize and deserialze
// inventory information from unstructured ConfigMaps and Inventory objects.
// SatusPolicy is stored within the converter as ConfigMaps do not support
// the status sub-resource and we need to handle the status conversion to
// the data field.
type ConfigMapConverter struct {
	StatusPolicy StatusPolicy
}

var _ Converter = ConfigMapConverter{}

// GroupVersionKind returns the GVK supported by this Converter
func (cmc ConfigMapConverter) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}
}

// To converts from an Unstructured ConfigMap to an Inventory.
func (cmc ConfigMapConverter) To(obj *unstructured.Unstructured) (*actuation.Inventory, error) {
	inv := &actuation.Inventory{}
	// Copy TypeMeta
	inv.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	// Copy ObjectMeta
	object.DeepCopyObjectMetaInto(obj, inv)

	// Convert in.Data to out.Spec.Objects.
	data, _, err := unstructured.NestedStringMap(obj.UnstructuredContent(), "data")
	if err != nil {
		return nil, fmt.Errorf("failed to read ConfigMap data: %w", err)
	}
	objs, err := object.FromStringMap(data)
	if err != nil {
		return nil, err
	}
	inv.Spec.Objects = ObjectReferencesFromObjMetadataSet(objs)
	// Sort objects to reduce chun on update.
	sort.Sort(AlphanumericObjectReferences(inv.Spec.Objects))

	// // Convert in.Data to out.status.objects
	// var statuses []actuation.ObjectStatus
	// if len(objs) > 0 {

	// }

	// inv.Status.Objects = statuses

	return inv, nil
}

// From converts from an Inventory to an Unstructured ConfigMap.
func (cmc ConfigMapConverter) From(inv *actuation.Inventory) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	// Copy TypeMeta
	obj.SetGroupVersionKind(inv.GetObjectKind().GroupVersionKind())
	// Copy ObjectMeta
	object.DeepCopyObjectMetaInto(inv, obj)

	// Convert in.Spec.Objects to out.Data
	if len(inv.Spec.Objects) > 0 {
		ids := ObjMetadataSetFromObjectReferences(inv.Spec.Objects)
		var data map[string]string
		if cmc.StatusPolicy == StatusPolicyAll {
			data = buildObjMap(ids, inv.Status.Objects)
		} else {
			data = ids.ToStringMap()
		}

		err := unstructured.SetNestedStringMap(obj.Object, data, "data")
		if err != nil {
			return obj, fmt.Errorf("failed to update ConfigMap data: %w", err)
		}
	}

	return obj, nil
}

func buildObjMap(objMetas object.ObjMetadataSet, objStatus []actuation.ObjectStatus) map[string]string {
	objMap := map[string]string{}
	objStatusMap := map[object.ObjMetadata]actuation.ObjectStatus{}
	for _, status := range objStatus {
		objStatusMap[ObjMetadataFromObjectReference(status.ObjectReference)] = status
	}
	for _, objMetadata := range objMetas {
		if status, found := objStatusMap[objMetadata]; found {
			objMap[objMetadata.String()] = stringFrom(status)
		} else {
			// It's possible that the passed in status doesn't any object status
			objMap[objMetadata.String()] = ""
		}
	}
	return objMap
}

func stringFrom(status actuation.ObjectStatus) string {
	tmp := map[string]string{
		"strategy":  status.Strategy.String(),
		"actuation": status.Actuation.String(),
		"reconcile": status.Reconcile.String(),
	}
	data, err := json.Marshal(tmp)
	if err != nil || string(data) == "{}" {
		return ""
	}
	return string(data)
}

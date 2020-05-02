// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code for a "inventory" object which
// stores object metadata to keep track of sets of
// resources. This "inventory" object must be a ConfigMap
// and it stores the object metadata in the data field
// of the ConfigMap. By storing metadata from all applied
// objects, we can correctly prune and teardown sets
// of resources.

package prune

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// RetrieveInventoryLabel returns the string value of the InventoryLabel
// for the passed object. Returns error if the passed object is nil or
// is not a inventory object.
func RetrieveInventoryLabel(obj runtime.Object) (string, error) {
	var inventoryLabel string
	if obj == nil {
		return "", fmt.Errorf("inventory object is nil")
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return "", err
	}
	labels := accessor.GetLabels()
	inventoryLabel, exists := labels[common.InventoryLabel]
	if !exists {
		return "", fmt.Errorf("inventory label does not exist for inventory object: %s", common.InventoryLabel)
	}
	return strings.TrimSpace(inventoryLabel), nil
}

// IsInventoryObject returns true if the passed object has the
// inventory label.
// TODO(seans3): Check type is ConfigMap.
func IsInventoryObject(obj runtime.Object) bool {
	if obj == nil {
		return false
	}
	inventoryLabel, err := RetrieveInventoryLabel(obj)
	if err == nil && len(inventoryLabel) > 0 {
		return true
	}
	return false
}

// FindInventoryObj returns the "Inventory" object (ConfigMap with
// inventory label) if it exists, and a boolean describing if it was found.
func FindInventoryObj(infos []*resource.Info) (*resource.Info, bool) {
	for _, info := range infos {
		if info != nil && IsInventoryObject(info.Object) {
			return info, true
		}
	}
	return nil, false
}

// Adds the metadata of all objects (passed as infos) to the
// inventory object. Returns an error if a inventory object does not
// exist, or we are unable to successfully add the metadata to
// the inventory object; nil otherwise. Each object is in
// unstructured.Unstructured format.
func addObjsToInventory(infos []*resource.Info) error {
	var inventoryInfo *resource.Info
	var inventoryObj *unstructured.Unstructured
	objMap := map[string]string{}
	for _, info := range infos {
		obj := info.Object
		if IsInventoryObject(obj) {
			// If we have more than one inventory object--error.
			if inventoryObj != nil {
				return fmt.Errorf("error--applying more than one inventory object")
			}
			var ok bool
			inventoryObj, ok = obj.(*unstructured.Unstructured)
			if !ok {
				return fmt.Errorf("inventory object is not an Unstructured: %#v", inventoryObj)
			}
			inventoryInfo = info
		} else {
			if obj == nil {
				return fmt.Errorf("creating inventory; object is nil")
			}
			gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
			objMetadata, err := object.CreateObjMetadata(info.Namespace, info.Name, gk)
			if err != nil {
				return err
			}
			objMap[objMetadata.String()] = ""
		}
	}

	// If we've found the inventory object, store the object metadata inventory
	// in the inventory config map.
	if inventoryObj == nil {
		return fmt.Errorf("inventory object not found")
	}

	if len(objMap) > 0 {
		// Adds the inventory map to the ConfigMap "data" section.
		err := unstructured.SetNestedStringMap(inventoryObj.UnstructuredContent(),
			objMap, "data")
		if err != nil {
			return err
		}
		// Adds the hash of the obj metadata strings as an annotation to the
		// inventory object. Object metadata strings must be sorted to make hash
		// deterministic.
		objList := mapKeysToSlice(objMap)
		sort.Strings(objList)
		objsHash, err := calcInventoryHash(objList)
		if err != nil {
			return err
		}
		// Add the hash as a suffix to the inventory object's name.
		objsHashStr := strconv.FormatUint(uint64(objsHash), 16)
		if err := addSuffixToName(inventoryInfo, objsHashStr); err != nil {
			return err
		}
		annotations := inventoryObj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[common.InventoryHash] = objsHashStr
		inventoryObj.SetAnnotations(annotations)
	}
	return nil
}

// CreateInventoryObj creates an inventory object based on a inventory object
// template. The passed "resources" parameter are applied at the same time
// as the inventory object, and metadata for each is stored in the inventory
// object.
func CreateInventoryObj(inventoryTemplate *resource.Info,
	resources []*resource.Info) (*resource.Info, error) {
	// Verify that the provided inventoryTemplate represents an
	// actual resource in the Unstructured format.
	obj := inventoryTemplate.Object
	if obj == nil {
		return nil, fmt.Errorf("inventory template has nil Object")
	}
	iot, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf(
			"inventory template is not an Unstructured: %s",
			inventoryTemplate.Source)
	}

	// Create the objMap of all the resources.
	objMap, err := buildObjMap(resources)
	if err != nil {
		return nil, err
	}

	invHashStr, err := computeInventoryHash(objMap)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%s", iot.GetName(), invHashStr)

	// Create the inventory object by copying the template.
	inventoryObj := iot.DeepCopy()
	inventoryObj.SetName(name)
	// Adds the inventory map to the ConfigMap "data" section.
	err = unstructured.SetNestedStringMap(inventoryObj.UnstructuredContent(),
		objMap, "data")
	if err != nil {
		return nil, err
	}
	annotations := inventoryObj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[common.InventoryHash] = invHashStr
	inventoryObj.SetAnnotations(annotations)

	// Creates a new Info for the newly created inventory object.
	return &resource.Info{
		Client:    inventoryTemplate.Client,
		Mapping:   inventoryTemplate.Mapping,
		Source:    "generated",
		Name:      inventoryObj.GetName(),
		Namespace: inventoryObj.GetNamespace(),
		Object:    inventoryObj,
	}, nil
}

func buildObjMap(resources []*resource.Info) (map[string]string, error) {
	objMap := map[string]string{}
	for _, res := range resources {
		if res.Object == nil {
			return nil, fmt.Errorf("creating obj metadata; object is nil")
		}
		obj := res.Object
		gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
		objMetadata, err := object.CreateObjMetadata(res.Namespace,
			res.Name, gk)
		if err != nil {
			return nil, err
		}
		objMap[objMetadata.String()] = ""
	}
	return objMap, nil
}

func computeInventoryHash(objMap map[string]string) (string, error) {
	objList := mapKeysToSlice(objMap)
	sort.Strings(objList)
	invHash, err := calcInventoryHash(objList)
	if err != nil {
		return "", err
	}
	// Compute the name of the inventory object. It is the name of the
	// inventory object template that it is based on with an additional
	// suffix which is based on the hash of the inventory.
	return strconv.FormatUint(uint64(invHash), 16), nil
}

// RetrieveObjsFromInventoryObj returns a slice of pointers to the
// object metadata. This function finds the inventory object, then
// parses the stored resource metadata into ObjMetadata structs. Returns
// an error if there is a problem parsing the data into ObjMetadata
// structs, or if the inventory object is not in Unstructured format; nil
// otherwise. If a inventory object does not exist, or it does not have a
// "data" map, then returns an empty slice and no error.
func RetrieveObjsFromInventory(infos []*resource.Info) ([]*object.ObjMetadata, error) {
	objs := []*object.ObjMetadata{}
	inventoryInfo, exists := FindInventoryObj(infos)
	if exists {
		inventoryObj, ok := inventoryInfo.Object.(*unstructured.Unstructured)
		if !ok {
			err := fmt.Errorf("inventory object is not an Unstructured: %#v", inventoryObj)
			return objs, err
		}
		objMap, exists, err := unstructured.NestedStringMap(inventoryObj.Object, "data")
		if err != nil {
			err := fmt.Errorf("error retrieving object metadata from inventory object")
			return objs, err
		}
		if exists {
			for objStr := range objMap {
				obj, err := object.ParseObjMetadata(objStr)
				if err != nil {
					return objs, err
				}
				objs = append(objs, obj)
			}
		}
	}
	return objs, nil
}

// ClearInventoryObj finds the inventory object in the list of objects,
// and sets an empty inventory. Returns error if the inventory object
// is not Unstructured, the inventory object does not exist, or if
// we can't set the empty inventory on the inventory object. If successful,
// returns nil.
func ClearInventoryObj(infos []*resource.Info) error {
	// Initially, find the inventory object ConfigMap (in Unstructured format).
	var inventoryObj *unstructured.Unstructured
	for _, info := range infos {
		obj := info.Object
		if IsInventoryObject(obj) {
			var ok bool
			inventoryObj, ok = obj.(*unstructured.Unstructured)
			if !ok {
				return fmt.Errorf("inventory object is not an Unstructured: %#v", inventoryObj)
			}
			break
		}
	}
	if inventoryObj == nil {
		return fmt.Errorf("inventory object not found")
	}
	// Clears the inventory map of the ConfigMap "data" section.
	emptyMap := map[string]string{}
	err := unstructured.SetNestedStringMap(inventoryObj.UnstructuredContent(),
		emptyMap, "data")
	if err != nil {
		return err
	}

	return nil
}

// calcInventoryHash returns an unsigned int32 representing the hash
// of the obj metadata strings. If there is an error writing bytes to
// the hash, then the error is returned; nil is returned otherwise.
// Used to quickly identify the set of resources in the inventory object.
func calcInventoryHash(inv []string) (uint32, error) {
	h := fnv.New32a()
	for _, is := range inv {
		_, err := h.Write([]byte(is))
		if err != nil {
			return uint32(0), err
		}
	}
	return h.Sum32(), nil
}

// retrieveInventoryHash takes a inventory object (encapsulated by
// a resource.Info), and returns the string representing the hash
// of the set of obj metadata; returns empty string if the inventory
// object is not in Unstructured format, or if the hash annotation
// does not exist.
func retrieveInventoryHash(inventoryInfo *resource.Info) string {
	var invHash = ""
	inventoryObj, ok := inventoryInfo.Object.(*unstructured.Unstructured)
	if ok {
		annotations := inventoryObj.GetAnnotations()
		if annotations != nil {
			invHash = annotations[common.InventoryHash]
		}
	}
	return invHash
}

// mapKeysToSlice returns the map keys as a slice of strings.
func mapKeysToSlice(m map[string]string) []string {
	s := make([]string, len(m))
	i := 0
	for k := range m {
		s[i] = k
		i++
	}
	return s
}

// addSuffixToName adds the passed suffix (usually a hash) as a suffix
// to the name of the passed object stored in the Info struct. Returns
// an error if the object is not "*unstructured.Unstructured" or if the
// name stored in the object differs from the name in the Info struct.
func addSuffixToName(info *resource.Info, suffix string) error {
	if info == nil {
		return fmt.Errorf("nil resource.Info")
	}
	suffix = strings.TrimSpace(suffix)
	if len(suffix) == 0 {
		return fmt.Errorf("passed empty suffix")
	}

	accessor, _ := meta.Accessor(info.Object)
	name := accessor.GetName()
	if name != info.Name {
		return fmt.Errorf("inventory object (%s) and resource.Info (%s) have different names", name, info.Name)
	}
	// Error if name alread has suffix.
	suffix = "-" + suffix
	if strings.HasSuffix(name, suffix) {
		return fmt.Errorf("name already has suffix: %s", name)
	}
	name += suffix
	accessor.SetName(name)
	info.Name = name

	return nil
}

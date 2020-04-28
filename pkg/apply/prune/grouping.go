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

// retrieveInventoryLabel returns the string value of the InventoryLabel
// for the passed object. Returns error if the passed object is nil or
// is not a inventory object.
func retrieveInventoryLabel(obj runtime.Object) (string, error) {
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
	inventoryLabel, err := retrieveInventoryLabel(obj)
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

// Adds the inventory of all objects (passed as infos) to the
// grouping object. Returns an error if a grouping object does not
// exist, or we are unable to successfully add the inventory to
// the grouping object; nil otherwise. Each object is in
// unstructured.Unstructured format.
func AddInventoryToGroupingObj(infos []*resource.Info) error {
	// Iterate through the objects (infos), creating an Inventory struct
	// as metadata for each object, or if it's the grouping object, store it.
	var groupingInfo *resource.Info
	var groupingObj *unstructured.Unstructured
	inventoryMap := map[string]string{}
	for _, info := range infos {
		obj := info.Object
		if IsInventoryObject(obj) {
			// If we have more than one grouping object--error.
			if groupingObj != nil {
				return fmt.Errorf("error--applying more than one grouping object")
			}
			var ok bool
			groupingObj, ok = obj.(*unstructured.Unstructured)
			if !ok {
				return fmt.Errorf("grouping object is not an Unstructured: %#v", groupingObj)
			}
			groupingInfo = info
		} else {
			if obj == nil {
				return fmt.Errorf("creating inventory; object is nil")
			}
			gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
			objMetadata, err := object.CreateObjMetadata(info.Namespace, info.Name, gk)
			if err != nil {
				return err
			}
			inventoryMap[objMetadata.String()] = ""
		}
	}

	// If we've found the grouping object, store the object metadata inventory
	// in the grouping config map.
	if groupingObj == nil {
		return fmt.Errorf("grouping object not found")
	}

	if len(inventoryMap) > 0 {
		// Adds the inventory map to the ConfigMap "data" section.
		err := unstructured.SetNestedStringMap(groupingObj.UnstructuredContent(),
			inventoryMap, "data")
		if err != nil {
			return err
		}
		// Adds the hash of the inventory strings as an annotation to the
		// grouping object. Inventory strings must be sorted to make hash
		// deterministic.
		inventoryList := mapKeysToSlice(inventoryMap)
		sort.Strings(inventoryList)
		invHash, err := calcInventoryHash(inventoryList)
		if err != nil {
			return err
		}
		// Add the hash as a suffix to the grouping object's name.
		invHashStr := strconv.FormatUint(uint64(invHash), 16)
		if err := addSuffixToName(groupingInfo, invHashStr); err != nil {
			return err
		}
		annotations := groupingObj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[common.InventoryHash] = invHashStr
		groupingObj.SetAnnotations(annotations)
	}
	return nil
}

// CreateGroupingObj creates a grouping object based on a grouping object
// template and the set of resources that will be in the inventory.
func CreateGroupingObj(groupingObjectTemplate *resource.Info,
	resources []*resource.Info) (*resource.Info, error) {
	// Verify that the provided groupingObjectTemplate represents an
	// actual resource in the Unstructured format.
	obj := groupingObjectTemplate.Object
	if obj == nil {
		return nil, fmt.Errorf("grouping object template has nil Object")
	}
	got, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf(
			"grouping object template is not an Unstructured: %s",
			groupingObjectTemplate.Source)
	}

	// Create the inventoryMap of all the resources.
	inventoryMap, err := buildInventoryMap(resources)
	if err != nil {
		return nil, err
	}

	invHashStr, err := computeInventoryHash(inventoryMap)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%s", got.GetName(), invHashStr)

	// Create the grouping object by copying the template.
	groupingObj := got.DeepCopy()
	groupingObj.SetName(name)
	// Adds the inventory map to the ConfigMap "data" section.
	err = unstructured.SetNestedStringMap(groupingObj.UnstructuredContent(),
		inventoryMap, "data")
	if err != nil {
		return nil, err
	}
	annotations := groupingObj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[common.InventoryHash] = invHashStr
	groupingObj.SetAnnotations(annotations)

	// Creates a new Info for the newly created grouping object.
	return &resource.Info{
		Client:    groupingObjectTemplate.Client,
		Mapping:   groupingObjectTemplate.Mapping,
		Source:    "generated",
		Name:      groupingObj.GetName(),
		Namespace: groupingObj.GetNamespace(),
		Object:    groupingObj,
	}, nil
}

func buildInventoryMap(resources []*resource.Info) (map[string]string, error) {
	inventoryMap := map[string]string{}
	for _, res := range resources {
		if res.Object == nil {
			return nil, fmt.Errorf("creating inventory; object is nil")
		}
		obj := res.Object
		gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
		objMetadata, err := object.CreateObjMetadata(res.Namespace,
			res.Name, gk)
		if err != nil {
			return nil, err
		}
		inventoryMap[objMetadata.String()] = ""
	}
	return inventoryMap, nil
}

func computeInventoryHash(inventoryMap map[string]string) (string, error) {
	inventoryList := mapKeysToSlice(inventoryMap)
	sort.Strings(inventoryList)
	invHash, err := calcInventoryHash(inventoryList)
	if err != nil {
		return "", err
	}
	// Compute the name of the inventory object. It is the name of the
	// inventory object template that it is based on with an additional
	// suffix which is based on the hash of the inventory.
	return strconv.FormatUint(uint64(invHash), 16), nil
}

// RetrieveInventoryFromGroupingObj returns a slice of pointers to the
// inventory metadata. This function finds the grouping object, then
// parses the stored resource metadata into Inventory structs. Returns
// an error if there is a problem parsing the data into Inventory
// structs, or if the grouping object is not in Unstructured format; nil
// otherwise. If a grouping object does not exist, or it does not have a
// "data" map, then returns an empty slice and no error.
func RetrieveInventoryFromGroupingObj(infos []*resource.Info) ([]*object.ObjMetadata, error) {
	inventory := []*object.ObjMetadata{}
	groupingInfo, exists := FindInventoryObj(infos)
	if exists {
		groupingObj, ok := groupingInfo.Object.(*unstructured.Unstructured)
		if !ok {
			err := fmt.Errorf("grouping object is not an Unstructured: %#v", groupingObj)
			return inventory, err
		}
		invMap, exists, err := unstructured.NestedStringMap(groupingObj.Object, "data")
		if err != nil {
			err := fmt.Errorf("error retrieving inventory from grouping object")
			return inventory, err
		}
		if exists {
			for invStr := range invMap {
				inv, err := object.ParseObjMetadata(invStr)
				if err != nil {
					return inventory, err
				}
				inventory = append(inventory, inv)
			}
		}
	}
	return inventory, nil
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
// of the inventory strings. If there is an error writing bytes to
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
// of the inventory set; returns empty string if the inventory
// object is not in Unstructured format, or if the hash annotation
// does not exist.
func retrieveInventoryHash(groupingInfo *resource.Info) string {
	var invHash = ""
	groupingObj, ok := groupingInfo.Object.(*unstructured.Unstructured)
	if ok {
		annotations := groupingObj.GetAnnotations()
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
		return fmt.Errorf("grouping object (%s) and resource.Info (%s) have different names", name, info.Name)
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

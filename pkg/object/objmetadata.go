// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// ObjMetadata is the minimal set of information to
// uniquely identify an object. The four fields are:
//
//   Group/Kind (NOTE: NOT version)
//   Namespace
//   Name
//
// We specifically do not use the "version", because
// the APIServer does not recognize a version as a
// different resource. This metadata is used to identify
// resources for pruning and teardown.

package object

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/cli-runtime/pkg/resource"
)

// Separates inventory fields. This string is allowable as a
// ConfigMap key, but it is not allowed as a character in
// resource name.
const fieldSeparator = "_"

// ObjMetadata organizes and stores the indentifying information
// for an object. This struct (as a string) is stored in a
// inventory object to keep track of sets of applied objects.
type ObjMetadata struct {
	Namespace string
	Name      string
	GroupKind schema.GroupKind
}

// CreateObjMetadata returns a pointer to an ObjMetadata struct filled
// with the passed values. This function normalizes and validates the
// passed fields and returns an error for bad parameters.
func CreateObjMetadata(namespace string, name string, gk schema.GroupKind) (ObjMetadata, error) {
	// Namespace can be empty, but name cannot.
	name = strings.TrimSpace(name)
	if name == "" {
		return ObjMetadata{}, fmt.Errorf("empty name for object")
	}
	// Manually validate name, since by the time k8s reports the error
	// the invalid name has already been encoded into the inventory object.
	if !validateNameChars(name) {
		return ObjMetadata{}, fmt.Errorf("invalid characters in object name: %s", name)
	}
	if gk.Empty() {
		return ObjMetadata{}, fmt.Errorf("empty GroupKind for object")
	}
	return ObjMetadata{
		Namespace: strings.TrimSpace(namespace),
		Name:      name,
		GroupKind: gk,
	}, nil
}

// validateNameChars returns false if the passed name string contains
// any invalid characters; true otherwise. The allowed characters for
// a Kubernetes resource name are:
//
//   Most resource types require a name that can be used as a DNS label name
//   as defined in RFC 1123. This means the name must:
//
//   * contain no more than 253 characters
//   * contain only lowercase alphanumeric characters, '-'
//   * start with an alphanumeric character
//   * end with an alphanumeric character
//
func validateNameChars(name string) bool {
	errs := validation.IsDNS1123Subdomain(name)
	return len(errs) == 0
}

// ParseObjMetadata takes a string, splits it into its five fields,
// and returns a pointer to an ObjMetadata struct storing the
// five fields. Example inventory string:
//
//   test-namespace_test-name_apps_ReplicaSet
//
// Returns an error if unable to parse and create the ObjMetadata
// struct.
func ParseObjMetadata(inv string) (ObjMetadata, error) {
	parts := strings.Split(inv, fieldSeparator)
	if len(parts) == 4 {
		gk := schema.GroupKind{
			Group: strings.TrimSpace(parts[2]),
			Kind:  strings.TrimSpace(parts[3]),
		}
		return CreateObjMetadata(parts[0], parts[1], gk)
	}
	return ObjMetadata{}, fmt.Errorf("unable to decode inventory: %s", inv)
}

// Equals compares two ObjMetadata and returns true if they are equal. This does
// not contain any special treatment for the extensions API group.
func (o *ObjMetadata) Equals(other *ObjMetadata) bool {
	if other == nil {
		return false
	}
	return *o == *other
}

// String create a string version of the ObjMetadata struct.
func (o *ObjMetadata) String() string {
	return fmt.Sprintf("%s%s%s%s%s%s%s",
		o.Namespace, fieldSeparator,
		o.Name, fieldSeparator,
		o.GroupKind.Group, fieldSeparator,
		o.GroupKind.Kind)
}

// BuildObjectMetadata returns object metadata (ObjMetadata) for the
// passed objects (infos).
func InfosToObjMetas(infos []*resource.Info) ([]ObjMetadata, error) {
	objMetas := []ObjMetadata{}
	for _, info := range infos {
		objMeta, err := InfoToObjMeta(info)
		if err != nil {
			return nil, err
		}
		objMetas = append(objMetas, objMeta)
	}
	return objMetas, nil
}

// InfoToObjMeta takes information from the provided info and
// returns an ObjMetadata that identifies the resource.
func InfoToObjMeta(info *resource.Info) (ObjMetadata, error) {
	if info == nil || info.Object == nil {
		return ObjMetadata{}, fmt.Errorf("attempting to transform info, but it is empty")
	}
	obj := info.Object
	gk := obj.GetObjectKind().GroupVersionKind().GroupKind()
	return CreateObjMetadata(info.Namespace, info.Name, gk)
}

// CalcHash returns a hash of the sorted strings from
// the object metadata, or an error if one occurred.
func Hash(objs []ObjMetadata) (string, error) {
	objStrs := []string{}
	for _, obj := range objs {
		objStrs = append(objStrs, obj.String())
	}
	hashInt, err := calcHash(objStrs)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(hashInt), 16), nil
}

// calcHash returns an unsigned int32 representing the hash
// of the obj metadata strings. If there is an error writing bytes to
// the hash, then the error is returned; nil is returned otherwise.
// Used to quickly identify the set of resources in the inventory object.
func calcHash(objs []string) (uint32, error) {
	sort.Strings(objs)
	h := fnv.New32a()
	for _, obj := range objs {
		_, err := h.Write([]byte(obj))
		if err != nil {
			return uint32(0), err
		}
	}
	return h.Sum32(), nil
}

// SetDiff returns the slice of objects that exist in "a", but
// do not exist in "b" (A - B).
func SetDiff(setA []ObjMetadata, setB []ObjMetadata) []ObjMetadata {
	// Create a map of the elements of A
	m := map[string]ObjMetadata{}
	for _, a := range setA {
		m[a.String()] = a
	}
	// Remove from A each element of B
	for _, b := range setB {
		delete(m, b.String()) // OK to delete even if b not in m
	}
	// Create/return slice from the map of remaining items
	diff := []ObjMetadata{}
	for _, r := range m {
		diff = append(diff, r)
	}
	return diff
}

// Union returns the slice of objects that is the set of unique
// items of the merging of set A and set B.
func Union(setA []ObjMetadata, setB []ObjMetadata) []ObjMetadata {
	m := map[string]ObjMetadata{}
	for _, a := range setA {
		m[a.String()] = a
	}
	for _, b := range setB {
		m[b.String()] = b
	}
	union := []ObjMetadata{}
	for _, u := range m {
		union = append(union, u)
	}
	return union
}

// SetEquals returns true if the slice of objects in setA equals
// the slice of objects in setB.
func SetEquals(setA []ObjMetadata, setB []ObjMetadata) bool {
	mapA := map[string]bool{}
	for _, a := range setA {
		mapA[a.String()] = true
	}
	mapB := map[string]bool{}
	for _, b := range setB {
		mapB[b.String()] = true
	}
	if len(mapA) != len(mapB) {
		return false
	}
	for b := range mapB {
		if _, exists := mapA[b]; !exists {
			return false
		}
	}
	return true
}

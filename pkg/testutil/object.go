// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// The testutil package houses utility function for testing.

package testutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var codec = scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)

// Unstructured translates the passed object config string into an
// object in Unstructured format. The mutators modify the config
// yaml before returning the object.
func Unstructured(t *testing.T, manifest string, mutators ...Mutator) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	err := runtime.DecodeInto(codec, []byte(manifest), u)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	for _, m := range mutators {
		m.Mutate(u)
	}
	return u
}

// Mutator inteface defines a function to update an object
// while translating it unto Unstructured format from yaml config.
type Mutator interface {
	Mutate(u *unstructured.Unstructured)
}

// ToIdentifer translates object yaml config into ObjMetadata.
func ToIdentifier(t *testing.T, manifest string) object.ObjMetadata {
	obj := Unstructured(t, manifest)
	return object.ObjMetadata{
		GroupKind: obj.GetObjectKind().GroupVersionKind().GroupKind(),
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(), // If cluster-scoped, empty namespace string
	}
}

// AddOwningInv returns a Mutator which adds the passed inv string
// as the owning inventory annotation.
func AddOwningInv(t *testing.T, inv string) Mutator {
	return owningInvMutator{
		t:   t,
		inv: inv,
	}
}

// owningInvMutator encapsulates the fields necessary to modify
// an object by adding the owning inventory annotation. This
// structure implements the Mutator interface.
type owningInvMutator struct {
	t   *testing.T
	inv string
}

// Mutate updates the passed object by adding the owning
// inventory annotation. Needed to implement the Mutator interface.
func (a owningInvMutator) Mutate(u *unstructured.Unstructured) {
	annos, found, err := unstructured.NestedStringMap(u.Object, "metadata", "annotations")
	if !assert.NoError(a.t, err) {
		a.t.FailNow()
	}
	if !found {
		annos = make(map[string]string)
	}
	annos["config.k8s.io/owning-inventory"] = a.inv
	err = unstructured.SetNestedStringMap(u.Object, annos, "metadata", "annotations")
	if !assert.NoError(a.t, err) {
		a.t.FailNow()
	}
}

// AddDependsOn returns a Mutator which adds the passed objects as a
// depends-on annotation to the object which is mutated. Multiple objects
// passed in means multiple depends on objects in the annotation separated
// by a comma.
func AddDependsOn(t *testing.T, objs ...*unstructured.Unstructured) Mutator {
	return dependsOnMutator{
		t:       t,
		depObjs: objs,
	}
}

// dependsOnMutator encapsulates fields for adding depends-on annotation
// to a test object. Implements the Mutator interface.
type dependsOnMutator struct {
	t       *testing.T
	depObjs []*unstructured.Unstructured
}

// Mutate for dependsOnMutator adds the stored object dependencies
// in the depends-on annotation for the passed mutated object.
func (d dependsOnMutator) Mutate(u *unstructured.Unstructured) {
	// Add depends on object annotation to passed object.
	var objStr string
	// Iterate through all dependent objects to create the
	// depends-on annotation string.
	for i, depObj := range d.depObjs {
		if i > 0 {
			objStr += ","
		}
		groupKind := depObj.GroupVersionKind().GroupKind()
		group := groupKind.Group
		kind := groupKind.Kind
		name := depObj.GetName()
		if object.IsNamespaced(depObj) {
			objStr += fmt.Sprintf("%s/namespaces/%s/%s/%s",
				group, depObj.GetNamespace(), kind, name)
		} else {
			objStr += fmt.Sprintf("%s/%s/%s", group, kind, name)
		}
	}
	annos, found, err := unstructured.NestedStringMap(u.Object, "metadata", "annotations")
	if !assert.NoError(d.t, err) {
		d.t.FailNow()
	}
	if !found {
		annos = make(map[string]string)
	}
	annos[object.DependsOnAnnotation] = objStr
	err = unstructured.SetNestedStringMap(u.Object, annos, "metadata", "annotations")
	if !assert.NoError(d.t, err) {
		d.t.FailNow()
	}
}

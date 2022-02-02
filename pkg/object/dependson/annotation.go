// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package dependson

import (
	"errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/reference"
)

const (
	Annotation = "config.kubernetes.io/depends-on"
)

// HasAnnotation returns true if the config.kubernetes.io/depends-on annotation
// is present, false if not.
func HasAnnotation(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	_, found := u.GetAnnotations()[Annotation]
	return found
}

// ReadAnnotation reads the depends-on annotation and parses the the set of
// object references.
func ReadAnnotation(obj *unstructured.Unstructured) (DependencySet, error) {
	depSet := DependencySet{}
	if obj == nil {
		return depSet, nil
	}
	depSetStr, found := obj.GetAnnotations()[Annotation]
	if !found {
		return depSet, nil
	}
	if klog.V(5).Enabled() {
		klog.Infof("object (%v) has depends-on annotation: %s",
			reference.ObjectReferenceFromUnstructured(obj), depSetStr)
	}

	depSet, err := ParseDependentsSet(depSetStr)
	if err != nil {
		return depSet, object.InvalidAnnotationError{
			Annotation: Annotation,
			Cause:      err,
		}
	}
	return depSet, nil
}

// WriteAnnotation updates the supplied unstructured object to add the
// depends-on annotation.
func WriteAnnotation(obj *unstructured.Unstructured, depSet DependencySet) error {
	if obj == nil {
		return errors.New("object is nil")
	}
	if depSet.Equal(DependencySet{}) {
		return errors.New("dependent set is empty")
	}

	depSetStr := depSet.String()

	a := obj.GetAnnotations()
	if a == nil {
		a = map[string]string{}
	}
	a[Annotation] = depSetStr
	obj.SetAnnotations(a)
	return nil
}

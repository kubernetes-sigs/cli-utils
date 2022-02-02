// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package dependents

import (
	"errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/reference"
)

const (
	Annotation = "config.kubernetes.io/dependents"
)

// HasAnnotation returns true if the config.kubernetes.io/dependents annotation
// is present, false if not.
func HasAnnotation(u *unstructured.Unstructured) bool {
	if u == nil {
		return false
	}
	_, found := u.GetAnnotations()[Annotation]
	return found
}

// ReadAnnotation reads the dependents annotation and parses the the set of
// object references.
func ReadAnnotation(obj *unstructured.Unstructured) (DependentSet, error) {
	depSet := DependentSet{}
	if obj == nil {
		return depSet, nil
	}
	depSetStr, found := obj.GetAnnotations()[Annotation]
	if !found {
		return depSet, nil
	}

	if klog.V(5).Enabled() {
		klog.Infof("object (%v) has dependents annotation: %s",
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
// dependents annotation.
func WriteAnnotation(obj *unstructured.Unstructured, depSet DependentSet) error {
	if obj == nil {
		return errors.New("object is nil")
	}
	if depSet.Equal(DependentSet{}) {
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

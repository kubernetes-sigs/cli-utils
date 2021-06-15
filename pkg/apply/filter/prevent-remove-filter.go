// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/common"
)

// PreventRemoveFilter implements ValidationFilter interface to determine
// if an object should not be pruned (deleted) because of a
// "prevent remove" annotation.
type PreventRemoveFilter struct{}

// Name returns the preferred name for the filter. Usually
// used for logging.
func (prf PreventRemoveFilter) Name() string {
	return "PreventRemoveFilter"
}

// Filter returns true if the passed object should NOT be pruned (deleted)
// because the "prevent remove" annotation is present; otherwise returns
// false. Never returns an error.
func (prf PreventRemoveFilter) Filter(obj *unstructured.Unstructured) (bool, error) {
	for annotation, value := range obj.GetAnnotations() {
		if common.NoDeletion(annotation, value) {
			return true, nil
		}
	}
	return false, nil
}

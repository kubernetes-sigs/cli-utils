// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
)

// CurrentUIDFilter implements ValidationFilter interface to determine
// if an object should not be pruned (deleted) because it has recently
// been applied.
type CurrentUIDFilter struct {
	CurrentUIDs sets.String
}

// Name returns a filter identifier for logging.
func (cuf CurrentUIDFilter) Name() string {
	return "CurrentUIDFilter"
}

// Filter returns true if the passed object should NOT be pruned (deleted)
// because the it is a namespace that objects still reside in; otherwise
// returns false. This filter should not be added to the list of filters
// for "destroying", since every object is being deletet. Never returns an error.
func (cuf CurrentUIDFilter) Filter(obj *unstructured.Unstructured) (bool, error) {
	uid := string(obj.GetUID())
	if cuf.CurrentUIDs.Has(uid) {
		return true, nil
	}
	return false, nil
}

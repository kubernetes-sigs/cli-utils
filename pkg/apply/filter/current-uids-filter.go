// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"fmt"

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
func (cuf CurrentUIDFilter) Filter(obj *unstructured.Unstructured) (bool, string, error) {
	uid := string(obj.GetUID())
	if cuf.CurrentUIDs.Has(uid) {
		reason := fmt.Sprintf("object removal prevented; UID just applied: %s", uid)
		return true, reason, nil
	}
	return false, "", nil
}

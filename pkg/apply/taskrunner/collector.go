// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// newResourceStatusCollector returns a new resourceStatusCollector
// that will keep track of the status of the provided resources.
func newResourceStatusCollector(identifiers []object.ObjMetadata) *resourceStatusCollector {
	rm := make(map[object.ObjMetadata]resourceStatus)

	for _, obj := range identifiers {
		rm[obj] = resourceStatus{
			Identifier:    obj,
			CurrentStatus: status.UnknownStatus,
		}
	}
	return &resourceStatusCollector{
		resourceMap: rm,
	}
}

// resourceStatusCollector keeps track of the latest seen status for all the
// resources that is of interest during the operation.
type resourceStatusCollector struct {
	resourceMap map[object.ObjMetadata]resourceStatus
}

// resoureStatus contains the latest status for a given
// resource as identified by the Identifier.
type resourceStatus struct {
	Identifier    object.ObjMetadata
	CurrentStatus status.Status
}

// resourceStatus updates the collector with the latest
// seen status for the given resource.
func (a *resourceStatusCollector) resourceStatus(identifier object.ObjMetadata, s status.Status) {
	if ri, found := a.resourceMap[identifier]; found {
		ri.CurrentStatus = s
		a.resourceMap[identifier] = ri
	}
}

// conditionMet tests whether the provided Condition holds true for
// all resources given by the list of Identifiers.
func (a *resourceStatusCollector) conditionMet(identifiers []object.ObjMetadata, c condition) bool {
	switch c {
	case AllCurrent:
		return a.allMatchStatus(identifiers, status.CurrentStatus)
	case AllNotFound:
		return a.allMatchStatus(identifiers, status.NotFoundStatus)
	default:
		return a.noneMatchStatus(identifiers, status.UnknownStatus)
	}
}

// allMatchStatus checks whether all resources given by the
// Identifiers parameter has the provided status.
func (a *resourceStatusCollector) allMatchStatus(identifiers []object.ObjMetadata, s status.Status) bool {
	for id, ri := range a.resourceMap {
		if contains(identifiers, id) {
			if ri.CurrentStatus != s {
				return false
			}
		}
	}
	return true
}

// noneMatchStatus checks whether none of the resources given
// by the Identifiers parameters has the provided status.
func (a *resourceStatusCollector) noneMatchStatus(identifiers []object.ObjMetadata, s status.Status) bool {
	for id, ri := range a.resourceMap {
		if contains(identifiers, id) {
			if ri.CurrentStatus == s {
				return false
			}
		}
	}
	return true
}

// contains checks whether the given id exists in the given slice
// of Identifiers.
func contains(identifiers []object.ObjMetadata, id object.ObjMetadata) bool {
	for _, identifier := range identifiers {
		if identifier.Equals(&id) {
			return true
		}
	}
	return false
}

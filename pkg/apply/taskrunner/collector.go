// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
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
	Message       string
	Generation    int64
}

// resourceStatus updates the collector with the latest
// seen status for the given resource.
func (a *resourceStatusCollector) resourceStatus(r *event.ResourceStatus) {
	if ri, found := a.resourceMap[r.Identifier]; found {
		ri.CurrentStatus = r.Status
		ri.Message = r.Message
		ri.Generation = getGeneration(r)
		a.resourceMap[r.Identifier] = ri
	}
}

// getGeneration looks up the value of the generation field in the
// provided resource status. If the resource information is not available,
// this will return 0.
func getGeneration(r *event.ResourceStatus) int64 {
	if r.Resource == nil {
		return 0
	}
	return r.Resource.GetGeneration()
}

// conditionMet tests whether the provided Condition holds true for
// all resources given by the list of Identifiers.
func (a *resourceStatusCollector) conditionMet(rwd []resourceWaitData, c Condition) bool {
	switch c {
	case AllCurrent:
		return a.allMatchStatus(rwd, status.CurrentStatus)
	case AllNotFound:
		return a.allMatchStatus(rwd, status.NotFoundStatus)
	default:
		return a.noneMatchStatus(rwd, status.UnknownStatus)
	}
}

// allMatchStatus checks whether all resources given by the
// Identifiers parameter has the provided status.
func (a *resourceStatusCollector) allMatchStatus(rwd []resourceWaitData, s status.Status) bool {
	for _, wd := range rwd {
		ri, found := a.resourceMap[wd.identifier]
		if !found {
			return false
		}
		if ri.Generation < wd.generation || ri.CurrentStatus != s {
			return false
		}
	}
	return true
}

// noneMatchStatus checks whether none of the resources given
// by the Identifiers parameters has the provided status.
func (a *resourceStatusCollector) noneMatchStatus(rwd []resourceWaitData, s status.Status) bool {
	for _, wd := range rwd {
		ri, found := a.resourceMap[wd.identifier]
		if !found {
			return false
		}
		if ri.Generation < wd.generation || ri.CurrentStatus == s {
			return false
		}
	}
	return true
}

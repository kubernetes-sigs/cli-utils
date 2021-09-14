// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"sync"

	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Condition is a type that defines the types of conditions
// which a WaitTask can use.
type Condition string

const (
	// AllCurrent Condition means all the provided resources
	// has reached (and remains in) the Current status.
	AllCurrent Condition = "AllCurrent"

	// AllNotFound Condition means all the provided resources
	// has reached the NotFound status, i.e. they are all deleted
	// from the cluster.
	AllNotFound Condition = "AllNotFound"
)

// Meets returns true if the provided status meets the condition and
// false if it does not.
func (c Condition) Meets(s status.Status) bool {
	switch c {
	case AllCurrent:
		return s == status.CurrentStatus
	case AllNotFound:
		return s == status.NotFoundStatus
	default:
		return false
	}
}

type ResourceGeneration struct {
	Identifier object.ObjMetadata
	Generation int64
}

// NewResourceStatusCollector returns a new resourceStatusCollector
// that will keep track of the status of the provided resources.
func NewResourceStatusCollector() *ResourceStatusCollector {
	return &ResourceStatusCollector{
		resourceMap: make(map[object.ObjMetadata]ResourceStatus),
	}
}

// ResourceStatusCollector keeps track of the latest seen status for all the
// resources that is of interest during the operation.
type ResourceStatusCollector struct {
	resourceMap map[object.ObjMetadata]ResourceStatus
	// mu protects concurrent map access
	mu sync.Mutex
}

// resoureStatus contains the latest status for a given
// resource as identified by the Identifier.
type ResourceStatus struct {
	CurrentStatus status.Status
	Message       string
	Generation    int64
}

// Put updates the collector with the specified status
func (a *ResourceStatusCollector) Put(id object.ObjMetadata, rs ResourceStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resourceMap[id] = rs
}

// PutEventStatus updates the collector with the latest status from an
// ResourceStatus event.
func (a *ResourceStatusCollector) PutEventStatus(rs *event.ResourceStatus) {
	a.Put(rs.Identifier, ResourceStatus{
		CurrentStatus: rs.Status,
		Message:       rs.Message,
		Generation:    getGeneration(rs),
	})
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

// ConditionMet tests whether the provided Condition holds true for
// all resources given by the list of Ids.
func (a *ResourceStatusCollector) ConditionMet(rwd []ResourceGeneration, c Condition) bool {
	switch c {
	case AllCurrent:
		return a.allMatchStatus(rwd, status.CurrentStatus)
	case AllNotFound:
		return a.allMatchStatus(rwd, status.NotFoundStatus)
	default:
		return a.noneMatchStatus(rwd, status.UnknownStatus)
	}
}

// matchStatus returns the status of any resources with the specified
// identifiers that match the supplied status.
func (a *ResourceStatusCollector) Get(id object.ObjMetadata) ResourceStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	rs, found := a.resourceMap[id]
	if !found {
		return ResourceStatus{
			CurrentStatus: status.UnknownStatus,
		}
	}
	return rs
}

// allMatchStatus checks whether all resources given by the
// Ids parameter has the provided status.
func (a *ResourceStatusCollector) allMatchStatus(rwd []ResourceGeneration, s status.Status) bool {
	for _, wd := range rwd {
		rs := a.Get(wd.Identifier)
		if rs.Generation < wd.Generation || rs.CurrentStatus != s {
			return false
		}
	}
	return true
}

// noneMatchStatus checks whether none of the resources given
// by the Ids parameters has the provided status.
func (a *ResourceStatusCollector) noneMatchStatus(rwd []ResourceGeneration, s status.Status) bool {
	for _, wd := range rwd {
		rs := a.Get(wd.Identifier)
		if rs.Generation < wd.Generation || rs.CurrentStatus == s {
			return false
		}
	}
	return true
}

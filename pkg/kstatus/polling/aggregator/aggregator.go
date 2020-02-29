// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// NewAllCurrentAggregator returns a new aggregator that will consider
// the set to be completed when all resources have reached the Current status.
func NewAllCurrentAggregator(identifiers []prune.ObjMetadata) *genericAggregator {
	return newGenericAggregator(identifiers, status.CurrentStatus)
}

// NewAllNotFoundAggregator returns a new aggregator that will consider
// the set to be completed when all resources have reached the NotFound status.
func NewAllNotFoundAggregator(identifiers []prune.ObjMetadata) *genericAggregator {
	return newGenericAggregator(identifiers, status.NotFoundStatus)
}

// genericAggregator implements the StatusAggregator interface. It can
// be customized by providing the desired status that all resources
// should reach.
type genericAggregator struct {
	resourceCurrentStatus map[prune.ObjMetadata]status.Status
	desiredStatus         status.Status
}

// newGenericAggregator returns a new Aggregator that will track the
// resources identified by the slice of ResourceIdentifiers. It will consider
// the change to be complete when all resources have reached the specified
// desired status.
func newGenericAggregator(identifiers []prune.ObjMetadata, desiredStatus status.Status) *genericAggregator {
	aggregator := &genericAggregator{
		resourceCurrentStatus: make(map[prune.ObjMetadata]status.Status),
		desiredStatus:         desiredStatus,
	}
	for _, id := range identifiers {
		aggregator.resourceCurrentStatus[id] = status.UnknownStatus
	}
	return aggregator
}

// ResourceStatus is called whenever we have an observation of a resource. In this
// case, we just keep the latest status so we can later compute the aggregate status
// for all the resources.
func (g *genericAggregator) ResourceStatus(r *event.ResourceStatus) {
	g.resourceCurrentStatus[r.Identifier] = r.Status
}

// AggregateStatus computes the aggregate status for all the resources.
// The rules are the following:
// - If any of the resources has the FailedStatus, the aggregate status is also
//   FailedStatus
// - If none of the resources have the FailedStatus and at least one is
//   UnknownStatus, the aggregate status is UnknownStatus
// - If all the resources have the desired status, the aggregate status is the
//   desired status.
// - If none of the first three rules apply, the aggregate status is
//   InProgressStatus
func (g *genericAggregator) AggregateStatus() status.Status {
	if len(g.resourceCurrentStatus) == 0 {
		return g.desiredStatus
	}

	allDesired := true
	anyUnknown := false
	for _, s := range g.resourceCurrentStatus {
		if s == status.FailedStatus {
			return status.FailedStatus
		}
		if s == status.UnknownStatus {
			anyUnknown = true
		}
		if s != g.desiredStatus {
			allDesired = false
		}
	}
	if anyUnknown {
		return status.UnknownStatus
	}
	if allDesired {
		return g.desiredStatus
	}
	return status.InProgressStatus
}

// Completed is used by the framework to decide if the set of resources has
// all reached the desired status, i.e. the aggregate status. This is used to determine
// when to stop polling resources.
func (g *genericAggregator) Completed() bool {
	return g.AggregateStatus() == g.desiredStatus
}

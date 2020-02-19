// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

// BasicAggregator implements StatusAggregator.
// Aggregate status will be Current when all observed
// resources are either Current or NotFound.
// TODO: Treating resources that doesn't exist as Current is
// weird. But it kinda does make sense when we track
// resources that are deleted/pruned. We should see if
// there is a better way to handle this.
type BasicAggregator struct {
	resourceCurrentStatus map[wait.ResourceIdentifier]status.Status
}

// NewBasicAggregator returns a BasicAggregator that will track
// resources identified by the argument.
func NewAllCurrentOrNotFoundStatusAggregator(identifiers []wait.ResourceIdentifier) *BasicAggregator {
	aggregator := &BasicAggregator{
		resourceCurrentStatus: make(map[wait.ResourceIdentifier]status.Status),
	}
	for _, id := range identifiers {
		aggregator.resourceCurrentStatus[id] = status.UnknownStatus
	}
	return aggregator
}

// ResourceObserved is called whenever we have an observation of a resource. In this
// case, we just keep the latest status so we can later compute the aggregate status
// for all the resources.
func (d *BasicAggregator) ResourceObserved(r *event.ObservedResource) {
	d.resourceCurrentStatus[r.Identifier] = r.Status
}

// AggregateStatus computes the aggregate status for all the resources. In this
// implementation of the Aggregator, we treat resources with the NotFound status as Current.
func (d *BasicAggregator) AggregateStatus() status.Status {
	// if we are not observing any resources, we consider status be Current.
	if len(d.resourceCurrentStatus) == 0 {
		return status.CurrentStatus
	}

	allCurrentOrNotFound := true
	anyUnknown := false
	for _, s := range d.resourceCurrentStatus {
		if s == status.FailedStatus {
			return status.FailedStatus
		}
		if s == status.UnknownStatus {
			anyUnknown = true
		}
		if !(s == status.CurrentStatus || s == status.NotFoundStatus) {
			allCurrentOrNotFound = false
		}
	}
	if anyUnknown {
		return status.UnknownStatus
	}
	if allCurrentOrNotFound {
		return status.CurrentStatus
	}
	return status.InProgressStatus
}

// Completed is used by the framework to decide if the set of resources has
// all reached the desired status, i.e. the aggregate status. This is used to determine
// when to stop polling resources.
func (d *BasicAggregator) Completed() bool {
	return d.AggregateStatus() == status.CurrentStatus
}

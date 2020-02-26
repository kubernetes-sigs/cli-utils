// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// StatusAggregator provides the interface the engine uses to compute the aggregate status over
// a set of statuses for individual resources. The implementation of this interface that should
// be used is set by providing an appropriate factory function when creating a PollerEngine.
// It also include a function that will be used by the engine to determine if all resources
// should be considered fully reconciled.
type StatusAggregator interface {
	// ResourceStatus notifies the aggregator of the status of a resource after it has been polled.
	ResourceStatus(resource *event.ResourceStatus)
	// AggregateStatus computes the aggregate status for all the resources at the given
	// point in time.
	AggregateStatus() status.Status
	// Completed returns true if all resources should be considered reconciled and false otherwise.
	Completed() bool
}

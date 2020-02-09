package observer

import (
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// StatusAggregator provides the interface the observer uses to compute the aggregate status.
// It also include a function that will be used by the observer to determine if all resources
// should be considered fully reconciled.
type StatusAggregator interface {
	// ResourceObserved notifies the aggregator of a new observation. Called after status has been
	// computed.
	ResourceObserved(resource *event.ObservedResource)
	// AggregateStatus computes the aggregate status for all the resources at the given
	// point in time.
	AggregateStatus() status.Status
	// Completed returns true if all resources should be considered reconciled and false otherwise.
	Completed() bool
}

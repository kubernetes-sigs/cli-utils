package observer

import (
	"context"
	"time"

	"github.com/go-errors/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AggregatorFactoryFunc defines the signature for the function the Observer will use to
// create a new StatusAggregator for each statusObserverRunner.
type AggregatorFactoryFunc func(identifiers []wait.ResourceIdentifier) StatusAggregator

// ReaderFactoryFunc defines the signature for the function the Observer will use to create
// a new ClusterReader for each statusObserverRunner.
type ReaderFactoryFunc func(reader client.Reader, mapper meta.RESTMapper,
	identifiers []wait.ResourceIdentifier) (ClusterReader, error)

// ObserversFactoryFunc defines the signature for the function the Observer will use to
// create the resource observers and the default observer for each statusObserverRunner.
type ObserversFactoryFunc func(reader ClusterReader, mapper meta.RESTMapper) (
	resourceObservers map[schema.GroupKind]ResourceObserver, defaultObserver ResourceObserver)

// Observer provides functionality for polling a cluster for status of a set of resources.
type Observer struct {
	Reader client.Reader
	Mapper meta.RESTMapper

	// AggregatorFunc provides the Observer with a way to create a new aggregator. This will
	// happen for every call to Observe since each statusObserverRunner keeps its own
	// status aggregator.
	AggregatorFactoryFunc AggregatorFactoryFunc

	// ReaderFactoryFunc provides the Observer with a factory function for creating new
	// ObserverReaders. Since these can be stateful, every call to Observe will create a new
	// ClusterReader.
	ReaderFactoryFunc ReaderFactoryFunc

	// ObserversFactoryFunc provides the Observer with a factory function for creating resource
	// observers. Each statusObserverRunner has a separate set of observers, so this will be called
	// for every call to Observe.
	ObserversFactoryFunc ObserversFactoryFunc
}

// Observe will create a new statusObserverRunner that will poll all the resources provided and report their status
// back on the event channel returned. The statusObserverRunner can be cancelled at any time by cancelling the
// context passed in.
// If observeUntilCancelled is set to false, then the runner will stop observing the resources when the StatusAggregator
// determines that all resources has been fully reconciled. If this is set to true, the observer will keep running
// until cancelled. This can be useful if the goal is to just monitor a set of resources rather than waiting for
// all to reach a specific status.
// The pollInterval specifies how often the observer should poll the cluster for the latest state of the resources.
func (s *Observer) Observe(ctx context.Context, identifiers []wait.ResourceIdentifier, pollInterval time.Duration,
	observeUntilCancelled bool) <-chan event.Event {
	eventChannel := make(chan event.Event)

	go func() {
		defer close(eventChannel)

		observerReader, err := s.ReaderFactoryFunc(s.Reader, s.Mapper, identifiers)
		if err != nil {
			eventChannel <- event.Event{
				EventType: event.ErrorEvent,
				Error:     errors.WrapPrefix(err, "error creating new ClusterReader", 1),
			}
			return
		}
		observers, defaultObserver := s.ObserversFactoryFunc(observerReader, s.Mapper)
		aggregator := s.AggregatorFactoryFunc(identifiers)

		runner := &statusObserverRunner{
			ctx:                       ctx,
			reader:                    observerReader,
			observers:                 observers,
			defaultObserver:           defaultObserver,
			identifiers:               identifiers,
			previousObservedResources: make(map[wait.ResourceIdentifier]*event.ObservedResource),
			eventChannel:              eventChannel,
			statusAggregator:          aggregator,
			observeUntilCancelled:     observeUntilCancelled,
			pollingInterval:           pollInterval,
		}
		runner.Run()
	}()

	return eventChannel
}

// statusObserverRunner is responsible for polling of a set of resources. Each call to Observe will create
// a new statusObserverRunner, which means we can keep state in the runner and all data will only be accessed
// by a single goroutine, meaning we don't need synchronization.
// The statusObserverRunner uses an implementation of the ClusterReader interface to talk to the
// kubernetes cluster. Currently this can be either the cached ClusterReader that syncs all needed resources
// with LIST calls before each polling loop, or the normal ClusterReader that just forwards each call
// to the client.Reader from controller-runtime.
type statusObserverRunner struct {
	// ctx is the context for the runner. It will be used by the caller of Observe to cancel
	// observing resources.
	ctx context.Context

	// reader is the interface for fetching and listing resources from the cluster. It can be implemented
	// to make call directly to the cluster or use caching to reduce the number of calls to the cluster.
	reader ClusterReader

	// observers contains the resource specific observers. These will contain logic for how to
	// compute status for specific GroupKinds. These will use an ClusterReader to fetch
	// status of a resource and any generated resources.
	observers map[schema.GroupKind]ResourceObserver

	// defaultObserver is the generic observer that is used for all GroupKinds that
	// doesn't have a specific observer in the observers map.
	defaultObserver ResourceObserver

	// identifiers contains the list of identifiers for the resources that should be observed.
	// Each resource is identified by GroupKind, namespace and name.
	identifiers []wait.ResourceIdentifier

	// previousObservedResources keeps track of the last event for each
	// of the observed resources. This is used to make sure we only
	// send events on the event channel when something has actually changed.
	previousObservedResources map[wait.ResourceIdentifier]*event.ObservedResource

	// eventChannel is a channel where any updates to the observed status of resources
	// will be sent. The caller of Observe will listen for updates.
	eventChannel chan event.Event

	// statusAggregator is responsible for keeping track of the status of
	// all of the observed resources and to compute the aggregate status.
	statusAggregator StatusAggregator

	// observeUntilCancelled decides whether the runner should keep running
	// even if the statusAggregator decides that all resources has reached the
	// desired status.
	observeUntilCancelled bool

	// pollingInterval determines how often we should poll the cluster for
	// the latest state of resources.
	pollingInterval time.Duration
}

// Run starts the polling loop of the observers.
func (r *statusObserverRunner) Run() {
	// Sets up ticker that will trigger the regular polling loop at a regular interval.
	ticker := time.NewTicker(r.pollingInterval)
	defer func() {
		ticker.Stop()
	}()

	for {
		select {
		case <-r.ctx.Done():
			// If the context has been cancelled, just send an AbortedEvent
			// and pass along the most up-to-date aggregate status. Then return
			// from this function, which will stop the ticker and close the event channel.
			aggregatedStatus := r.statusAggregator.AggregateStatus()
			r.eventChannel <- event.Event{
				EventType:       event.AbortedEvent,
				AggregateStatus: aggregatedStatus,
			}
			return
		case <-ticker.C:
			// First trigger a sync of the ClusterReader. This may or may not actually
			// result in calls to the cluster, depending on the implementation.
			// If this call fails, there is no clean way to recover, so we just return an ErrorEvent
			// and shut down.
			err := r.reader.Sync(r.ctx)
			if err != nil {
				r.eventChannel <- event.Event{
					EventType: event.ErrorEvent,
					Error:     err,
				}
				return
			}
			// Poll all resources and compute status. If the polling of resources has completed (based
			// on information from the StatusAggregator and the value of observeUntilCancelled), we send
			// a CompletedEvent and return.
			completed := r.observeStatusForAllResources()
			if completed {
				aggregatedStatus := r.statusAggregator.AggregateStatus()
				r.eventChannel <- event.Event{
					EventType:       event.CompletedEvent,
					AggregateStatus: aggregatedStatus,
				}
				return
			}
		}
	}
}

// observeStatusForAllResources iterates over all the resources in the set and delegates
// to the appropriate observer to compute the status.
func (r *statusObserverRunner) observeStatusForAllResources() bool {
	for _, id := range r.identifiers {
		gk := id.GroupKind
		observer := r.observerForGroupKind(gk)
		observedResource := observer.Observe(r.ctx, id)
		r.statusAggregator.ResourceObserved(observedResource)
		if r.isUpdatedObservedResource(observedResource) {
			r.previousObservedResources[id] = observedResource
			aggregatedStatus := r.statusAggregator.AggregateStatus()
			r.eventChannel <- event.Event{
				EventType:       event.ResourceUpdateEvent,
				AggregateStatus: aggregatedStatus,
				Resource:        observedResource,
			}
			if r.statusAggregator.Completed() && !r.observeUntilCancelled {
				return true
			}
		}
	}
	if r.statusAggregator.Completed() && !r.observeUntilCancelled {
		return true
	}
	return false
}

func (r *statusObserverRunner) observerForGroupKind(gk schema.GroupKind) ResourceObserver {
	observer, ok := r.observers[gk]
	if !ok {
		return r.defaultObserver
	}
	return observer
}

func (r *statusObserverRunner) isUpdatedObservedResource(observedResource *event.ObservedResource) bool {
	oldObservedResource, found := r.previousObservedResources[observedResource.Identifier]
	if !found {
		return true
	}
	return !event.ObservedStatusChanged(observedResource, oldObservedResource)
}

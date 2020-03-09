// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/go-errors/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AggregatorFactoryFunc defines the signature for the function the PollerEngine will use to
// create a new StatusAggregator for each statusPollerRunner.
type AggregatorFactoryFunc func(identifiers []object.ObjMetadata) StatusAggregator

// ClusterReaderFactoryFunc defines the signature for the function the PollerEngine will use to create
// a new ClusterReader for each statusPollerRunner.
type ClusterReaderFactoryFunc func(reader client.Reader, mapper meta.RESTMapper,
	identifiers []object.ObjMetadata) (ClusterReader, error)

// StatusReadersFactoryFunc defines the signature for the function the PollerEngine will use to
// create the resource statusReaders and the default engine for each statusPollerRunner.
type StatusReadersFactoryFunc func(reader ClusterReader, mapper meta.RESTMapper) (
	statusReaders map[schema.GroupKind]StatusReader, defaultStatusReader StatusReader)

// PollerEngine provides functionality for polling a cluster for status of a set of resources.
type PollerEngine struct {
	Reader client.Reader
	Mapper meta.RESTMapper
}

// Poll will create a new statusPollerRunner that will poll all the resources provided and report their status
// back on the event channel returned. The statusPollerRunner can be cancelled at any time by cancelling the
// context passed in.
// The context can be used to stop the polling process by using timeout, deadline or
// cancellation.
func (s *PollerEngine) Poll(ctx context.Context, identifiers []object.ObjMetadata, options Options) <-chan event.Event {
	eventChannel := make(chan event.Event)

	go func() {
		defer close(eventChannel)

		err := s.validate(options)
		if err != nil {
			eventChannel <- event.Event{
				EventType: event.ErrorEvent,
				Error:     err,
			}
			return
		}

		clusterReader, err := options.ClusterReaderFactoryFunc(s.Reader, s.Mapper, identifiers)
		if err != nil {
			eventChannel <- event.Event{
				EventType: event.ErrorEvent,
				Error:     errors.WrapPrefix(err, "error creating new ClusterReader", 1),
			}
			return
		}
		statusReaders, defaultStatusReader := options.StatusReadersFactoryFunc(clusterReader, s.Mapper)
		aggregator := options.AggregatorFactoryFunc(identifiers)

		runner := &statusPollerRunner{
			ctx:                      ctx,
			clusterReader:            clusterReader,
			statusReaders:            statusReaders,
			defaultStatusReader:      defaultStatusReader,
			identifiers:              identifiers,
			previousResourceStatuses: make(map[object.ObjMetadata]*event.ResourceStatus),
			eventChannel:             eventChannel,
			statusAggregator:         aggregator,
			pollUntilCancelled:       options.PollUntilCancelled,
			pollingInterval:          options.PollInterval,
		}
		runner.Run()
	}()

	return eventChannel
}

// validate checks that the passed in options contains valid values.
func (s *PollerEngine) validate(options Options) error {
	if options.ClusterReaderFactoryFunc == nil {
		return fmt.Errorf("clusterReaderFactoryFunc must be specified")
	}
	if options.AggregatorFactoryFunc == nil {
		return fmt.Errorf("aggregatorFactoryFunc must be specified")
	}
	if options.StatusReadersFactoryFunc == nil {
		return fmt.Errorf("statusReadersFactoryFunc must be specified")
	}
	return nil
}

// Options contains the different parameters that can be used to adjust the
// behavior of the PollerEngine.
// Timeout is not one of the options here as this should be accomplished by
// setting a timeout on the context: https://golang.org/pkg/context/
type Options struct {
	// PollUntilCancelled defines whether the engine should stop polling and close the
	// event channel when the Aggregator implementation considers all resources to have reached
	// the desired status.
	PollUntilCancelled bool

	// PollInterval defines how often the PollerEngine should poll the cluster for the latest
	// state of the resources.
	PollInterval time.Duration

	// AggregatorFunc provides the PollerEngine with a way to create a new aggregator. This will
	// happen for every call to Poll since each statusPollerRunner keeps its own
	// status aggregator.
	AggregatorFactoryFunc AggregatorFactoryFunc

	// ClusterReaderFactoryFunc provides the PollerEngine with a factory function for creating new
	// StatusReaders. Since these can be stateful, every call to Poll will create a new
	// ClusterReader.
	ClusterReaderFactoryFunc ClusterReaderFactoryFunc

	// StatusReadersFactoryFunc provides the PollerEngine with a factory function for creating new status
	// clusterReader. Each statusPollerRunner has a separate set of statusReaders, so this will be called
	// for every call to Poll.
	StatusReadersFactoryFunc StatusReadersFactoryFunc
}

// statusPollerRunner is responsible for polling of a set of resources. Each call to Poll will create
// a new statusPollerRunner, which means we can keep state in the runner and all data will only be accessed
// by a single goroutine, meaning we don't need synchronization.
// The statusPollerRunner uses an implementation of the ClusterReader interface to talk to the
// kubernetes cluster. Currently this can be either the cached ClusterReader that syncs all needed resources
// with LIST calls before each polling loop, or the normal ClusterReader that just forwards each call
// to the client.Reader from controller-runtime.
type statusPollerRunner struct {
	// ctx is the context for the runner. It will be used by the caller of Poll to cancel
	// polling resources.
	ctx context.Context

	// clusterReader is the interface for fetching and listing resources from the cluster. It can be implemented
	// to make call directly to the cluster or use caching to reduce the number of calls to the cluster.
	clusterReader ClusterReader

	// statusReaders contains the resource specific statusReaders. These will contain logic for how to
	// compute status for specific GroupKinds. These will use an ClusterReader to fetch
	// status of a resource and any generated resources.
	statusReaders map[schema.GroupKind]StatusReader

	// defaultStatusReader is the generic engine that is used for all GroupKinds that
	// doesn't have a specific engine in the statusReaders map.
	defaultStatusReader StatusReader

	// identifiers contains the list of identifiers for the resources that should be polled.
	// Each resource is identified by GroupKind, namespace and name.
	identifiers []object.ObjMetadata

	// previousResourceStatuses keeps track of the last event for each
	// of the polled resources. This is used to make sure we only
	// send events on the event channel when something has actually changed.
	previousResourceStatuses map[object.ObjMetadata]*event.ResourceStatus

	// eventChannel is a channel where any updates to the status of resources
	// will be sent. The caller of Poll will listen for updates.
	eventChannel chan event.Event

	// statusAggregator is responsible for keeping track of the status of
	// all of the resources and to compute the aggregate status.
	statusAggregator StatusAggregator

	// pollUntilCancelled decides whether the runner should keep running
	// even if the statusAggregator decides that all resources has reached the
	// desired status.
	pollUntilCancelled bool

	// pollingInterval determines how often we should poll the cluster for
	// the latest state of resources.
	pollingInterval time.Duration
}

// Run starts the polling loop of the statusReaders.
func (r *statusPollerRunner) Run() {
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
			err := r.clusterReader.Sync(r.ctx)
			if err != nil {
				r.eventChannel <- event.Event{
					EventType: event.ErrorEvent,
					Error:     err,
				}
				return
			}
			// Poll all resources and compute status. If the polling of resources has completed (based
			// on information from the StatusAggregator and the value of pollUntilCancelled), we send
			// a CompletedEvent and return.
			completed := r.pollStatusForAllResources()
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

// pollStatusForAllResources iterates over all the resources in the set and delegates
// to the appropriate engine to compute the status.
func (r *statusPollerRunner) pollStatusForAllResources() bool {
	for _, id := range r.identifiers {
		gk := id.GroupKind
		statusReader := r.statusReaderForGroupKind(gk)
		resourceStatus := statusReader.ReadStatus(r.ctx, id)
		r.statusAggregator.ResourceStatus(resourceStatus)
		if r.isUpdatedResourceStatus(resourceStatus) {
			r.previousResourceStatuses[id] = resourceStatus
			aggregatedStatus := r.statusAggregator.AggregateStatus()
			r.eventChannel <- event.Event{
				EventType:       event.ResourceUpdateEvent,
				AggregateStatus: aggregatedStatus,
				Resource:        resourceStatus,
			}
			if r.statusAggregator.Completed() && !r.pollUntilCancelled {
				return true
			}
		}
	}
	if r.statusAggregator.Completed() && !r.pollUntilCancelled {
		return true
	}
	return false
}

func (r *statusPollerRunner) statusReaderForGroupKind(gk schema.GroupKind) StatusReader {
	statusReader, ok := r.statusReaders[gk]
	if !ok {
		return r.defaultStatusReader
	}
	return statusReader
}

func (r *statusPollerRunner) isUpdatedResourceStatus(resourceStatus *event.ResourceStatus) bool {
	oldResourceStatus, found := r.previousResourceStatuses[resourceStatus.Identifier]
	if !found {
		return true
	}
	return !event.ResourceStatusChanged(resourceStatus, oldResourceStatus)
}

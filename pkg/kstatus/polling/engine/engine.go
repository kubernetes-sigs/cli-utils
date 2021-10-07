// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterReaderFactoryFunc defines the signature for the function the PollerEngine will use to create
// a new ClusterReader for each statusPollerRunner.
type ClusterReaderFactoryFunc func(reader client.Reader, mapper meta.RESTMapper,
	identifiers object.ObjMetadataSet) (ClusterReader, error)

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
func (s *PollerEngine) Poll(ctx context.Context, identifiers object.ObjMetadataSet, options Options) <-chan event.Event {
	eventChannel := make(chan event.Event)

	go func() {
		defer close(eventChannel)

		err := s.validate(options)
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		err = s.validateIdentifiers(identifiers)
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		clusterReader, err := options.ClusterReaderFactoryFunc(s.Reader, s.Mapper, identifiers)
		if err != nil {
			handleError(eventChannel, fmt.Errorf("error creating new ClusterReader: %w", err))
			return
		}
		statusReaders, defaultStatusReader := options.StatusReadersFactoryFunc(clusterReader, s.Mapper)

		runner := &statusPollerRunner{
			ctx:                      ctx,
			clusterReader:            clusterReader,
			statusReaders:            statusReaders,
			defaultStatusReader:      defaultStatusReader,
			identifiers:              identifiers,
			previousResourceStatuses: make(map[object.ObjMetadata]*event.ResourceStatus),
			eventChannel:             eventChannel,
			pollingInterval:          options.PollInterval,
		}
		runner.Run()
	}()

	return eventChannel
}

func handleError(eventChannel chan event.Event, err error) {
	eventChannel <- event.Event{
		EventType: event.ErrorEvent,
		Error:     err,
	}
}

// validate checks that the passed in options contains valid values.
func (s *PollerEngine) validate(options Options) error {
	if options.ClusterReaderFactoryFunc == nil {
		return fmt.Errorf("clusterReaderFactoryFunc must be specified")
	}
	if options.StatusReadersFactoryFunc == nil {
		return fmt.Errorf("statusReadersFactoryFunc must be specified")
	}
	return nil
}

// validateIdentifiers makes sure that all namespaced resources
// passed in
func (s *PollerEngine) validateIdentifiers(identifiers object.ObjMetadataSet) error {
	for _, id := range identifiers {
		mapping, err := s.Mapper.RESTMapping(id.GroupKind)
		if err != nil {
			// If we can't find a match, just keep going. This can happen
			// if CRDs and CRs are applied at the same time.
			if meta.IsNoMatchError(err) {
				continue
			}
			return err
		}
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace && id.Namespace == "" {
			return fmt.Errorf("resource %s %s is namespace scoped, but namespace is not set",
				id.GroupKind.String(), id.Name)
		}
	}
	return nil
}

// Options contains the different parameters that can be used to adjust the
// behavior of the PollerEngine.
// Timeout is not one of the options here as this should be accomplished by
// setting a timeout on the context: https://golang.org/pkg/context/
type Options struct {

	// PollInterval defines how often the PollerEngine should poll the cluster for the latest
	// state of the resources.
	PollInterval time.Duration

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

	// identifiers contains the set of identifiers for the resources that should be polled.
	// Each resource is identified by GroupKind, namespace and name.
	identifiers object.ObjMetadataSet

	// previousResourceStatuses keeps track of the last event for each
	// of the polled resources. This is used to make sure we only
	// send events on the event channel when something has actually changed.
	previousResourceStatuses map[object.ObjMetadata]*event.ResourceStatus

	// eventChannel is a channel where any updates to the status of resources
	// will be sent. The caller of Poll will listen for updates.
	eventChannel chan event.Event

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

	err := r.syncAndPoll()
	if err != nil {
		r.eventChannel <- event.Event{
			EventType: event.ErrorEvent,
			Error:     err,
		}
		return
	}

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			// First sync and then compute status for all resources.
			err := r.syncAndPoll()
			if err != nil {
				r.eventChannel <- event.Event{
					EventType: event.ErrorEvent,
					Error:     err,
				}
				return
			}
		}
	}
}

func (r *statusPollerRunner) syncAndPoll() error {
	// First trigger a sync of the ClusterReader. This may or may not actually
	// result in calls to the cluster, depending on the implementation.
	// If this call fails, there is no clean way to recover, so we just return an ErrorEvent
	// and shut down.
	err := r.clusterReader.Sync(r.ctx)
	if err != nil {
		return err
	}
	// Poll all resources and compute status. If the polling of resources has completed (based
	// on information from the StatusAggregator and the value of pollUntilCancelled), we send
	// a CompletedEvent and return.
	r.pollStatusForAllResources()
	return nil
}

// pollStatusForAllResources iterates over all the resources in the set and delegates
// to the appropriate engine to compute the status.
func (r *statusPollerRunner) pollStatusForAllResources() {
	for _, id := range r.identifiers {
		gk := id.GroupKind
		statusReader := r.statusReaderForGroupKind(gk)
		resourceStatus := statusReader.ReadStatus(r.ctx, id)
		if r.isUpdatedResourceStatus(resourceStatus) {
			r.previousResourceStatuses[id] = resourceStatus
			r.eventChannel <- event.Event{
				EventType: event.ResourceUpdateEvent,
				Resource:  resourceStatus,
			}
		}
	}
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
	return !event.ResourceStatusEqual(resourceStatus, oldResourceStatus)
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"sort"
	"sync"

	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

func NewObservedStatusCollector(identifiers []wait.ResourceIdentifier) *ObservedStatusCollector {
	observations := make(map[wait.ResourceIdentifier]*event.ObservedResource)
	for _, id := range identifiers {
		observations[id] = &event.ObservedResource{
			Identifier: id,
			Status:     status.UnknownStatus,
		}
	}
	return &ObservedStatusCollector{
		aggregateStatus: status.UnknownStatus,
		observations:    observations,
	}
}

// ObservedStatusCollector is for use by clients of the observe library and provides
// a way to keep track of the latest status/state for all the observed resources. The collector
// is set up to listen to the eventChannel and keep the latest event for each resource. It also
// provides a way to fetch the latest state for all resources and the aggregated status at any point.
// The functions already handles synchronization so it can be used by multiple goroutines.
type ObservedStatusCollector struct {
	mux sync.RWMutex

	lastEventType event.EventType

	aggregateStatus status.Status

	observations map[wait.ResourceIdentifier]*event.ObservedResource

	error error
}

// Observe kicks off the goroutine that will listen for the events on the eventChannel. It is also
// provided a stop channel that can be used by the caller to stop the collector before the
// eventChannel is closed. It returns a channel that will be closed the collector stops listening to
// the eventChannel.
func (o *ObservedStatusCollector) Observe(eventChannel <-chan event.Event, stop <-chan struct{}) <-chan struct{} {
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		for {
			select {
			case <-stop:
				return
			case event, more := <-eventChannel:
				if !more {
					return
				}
				o.processEvent(event)
			}
		}
	}()
	return completed
}

func (o *ObservedStatusCollector) processEvent(e event.Event) {
	o.mux.Lock()
	defer o.mux.Unlock()
	o.lastEventType = e.EventType
	if e.EventType == event.ErrorEvent {
		o.error = e.Error
		return
	}
	o.aggregateStatus = e.AggregateStatus
	if e.EventType == event.ResourceUpdateEvent {
		observedResource := e.Resource
		o.observations[observedResource.Identifier] = observedResource
	}
}

// Observation contains the latest state known by the collector as returned
// by a call to the LatestObservation function.
type Observation struct {
	LastEventType event.EventType

	AggregateStatus status.Status

	ObservedResources []*event.ObservedResource

	Error error
}

// LatestObservation returns an Observation instance, which contains the
// latest information about the resources known by the collector.
func (o *ObservedStatusCollector) LatestObservation() *Observation {
	o.mux.RLock()
	defer o.mux.RUnlock()

	var observedResources event.ObservedResources
	for _, observedResource := range o.observations {
		observedResources = append(observedResources, observedResource)
	}
	sort.Sort(observedResources)

	return &Observation{
		LastEventType:     o.lastEventType,
		AggregateStatus:   o.aggregateStatus,
		ObservedResources: observedResources,
		Error:             o.error,
	}
}

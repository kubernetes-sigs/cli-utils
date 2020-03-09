// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"sort"
	"sync"

	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func NewResourceStatusCollector(identifiers []object.ObjMetadata) *ResourceStatusCollector {
	resourceStatuses := make(map[object.ObjMetadata]*event.ResourceStatus)
	for _, id := range identifiers {
		resourceStatuses[id] = &event.ResourceStatus{
			Identifier: id,
			Status:     status.UnknownStatus,
		}
	}
	return &ResourceStatusCollector{
		aggregateStatus:  status.UnknownStatus,
		resourceStatuses: resourceStatuses,
	}
}

// ResourceStatusCollector is for use by clients of the polling library and provides
// a way to keep track of the latest status/state for all the polled resources. The collector
// is set up to listen to the eventChannel and keep the latest event for each resource. It also
// provides a way to fetch the latest state for all resources and the aggregated status at any point.
// The functions already handles synchronization so it can be used by multiple goroutines.
type ResourceStatusCollector struct {
	mux sync.RWMutex

	lastEventType event.EventType

	aggregateStatus status.Status

	resourceStatuses map[object.ObjMetadata]*event.ResourceStatus

	error error
}

// Listen kicks off the goroutine that will listen for the events on the eventChannel. It is also
// provided a stop channel that can be used by the caller to stop the collector before the
// eventChannel is closed. It returns a channel that will be closed the collector stops listening to
// the eventChannel.
func (o *ResourceStatusCollector) Listen(eventChannel <-chan event.Event, stop <-chan struct{}) <-chan struct{} {
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

func (o *ResourceStatusCollector) processEvent(e event.Event) {
	o.mux.Lock()
	defer o.mux.Unlock()
	o.lastEventType = e.EventType
	if e.EventType == event.ErrorEvent {
		o.error = e.Error
		return
	}
	o.aggregateStatus = e.AggregateStatus
	if e.EventType == event.ResourceUpdateEvent {
		resourceStatus := e.Resource
		o.resourceStatuses[resourceStatus.Identifier] = resourceStatus
	}
}

// Observation contains the latest state known by the collector as returned
// by a call to the LatestObservation function.
type Observation struct {
	LastEventType event.EventType

	AggregateStatus status.Status

	ResourceStatuses []*event.ResourceStatus

	Error error
}

// LatestObservation returns an Observation instance, which contains the
// latest information about the resources known by the collector.
func (o *ResourceStatusCollector) LatestObservation() *Observation {
	o.mux.RLock()
	defer o.mux.RUnlock()

	var resourceStatuses event.ResourceStatuses
	for _, resourceStatus := range o.resourceStatuses {
		resourceStatuses = append(resourceStatuses, resourceStatus)
	}
	sort.Sort(resourceStatuses)

	return &Observation{
		LastEventType:    o.lastEventType,
		AggregateStatus:  o.aggregateStatus,
		ResourceStatuses: resourceStatuses,
		Error:            o.error,
	}
}

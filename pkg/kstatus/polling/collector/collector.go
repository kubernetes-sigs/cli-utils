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
		ResourceStatuses: resourceStatuses,
	}
}

// Observer is an interface that can be implemented to have the
// ResourceStatusCollector invoke the function on every event that
// comes through the eventChannel.
// The callback happens in the processing goroutine and while the
// goroutine holds the lock, so any processing in the callback
// must be done quickly.
type Observer interface {
	Notify(*ResourceStatusCollector, event.Event)
}

// ObserverFunc is a function implementation of the Observer
// interface.
type ObserverFunc func(*ResourceStatusCollector, event.Event)

func (o ObserverFunc) Notify(rsc *ResourceStatusCollector, e event.Event) {
	o(rsc, e)
}

// ResourceStatusCollector is for use by clients of the polling library and provides
// a way to keep track of the latest status/state for all the polled resources. The collector
// is set up to listen to the eventChannel and keep the latest event for each resource. It also
// provides a way to fetch the latest state for all resources and the aggregated status at any point.
// The functions already handles synchronization so it can be used by multiple goroutines.
type ResourceStatusCollector struct {
	mux sync.RWMutex

	LastEventType event.EventType

	ResourceStatuses map[object.ObjMetadata]*event.ResourceStatus

	Error error
}

// Listen kicks off the goroutine that will listen for the events on the
// eventChannel.  It returns a channel that will be closed the collector stops
// listening to the eventChannel.
func (o *ResourceStatusCollector) Listen(eventChannel <-chan event.Event) <-chan struct{} {
	return o.ListenWithObserver(eventChannel, nil)
}

// Listen kicks off the goroutine that will listen for the events on the
// eventChannel.  It returns a channel that will be closed the collector stops
// listening to the eventChannel.
// The provided observer will be invoked on every event, after the event
// has been processed.
func (o *ResourceStatusCollector) ListenWithObserver(eventChannel <-chan event.Event,
	observer Observer) <-chan struct{} {
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		for e := range eventChannel {
			o.processEvent(e)
			if observer != nil {
				observer.Notify(o, e)
			}
		}
	}()
	return completed
}

func (o *ResourceStatusCollector) processEvent(e event.Event) {
	o.mux.Lock()
	defer o.mux.Unlock()
	o.LastEventType = e.EventType
	if e.EventType == event.ErrorEvent {
		o.Error = e.Error
		return
	}
	if e.EventType == event.ResourceUpdateEvent {
		resourceStatus := e.Resource
		o.ResourceStatuses[resourceStatus.Identifier] = resourceStatus
	}
}

// Observation contains the latest state known by the collector as returned
// by a call to the LatestObservation function.
type Observation struct {
	LastEventType event.EventType

	ResourceStatuses []*event.ResourceStatus

	Error error
}

// LatestObservation returns an Observation instance, which contains the
// latest information about the resources known by the collector.
func (o *ResourceStatusCollector) LatestObservation() *Observation {
	o.mux.RLock()
	defer o.mux.RUnlock()

	var resourceStatuses event.ResourceStatuses
	for _, resourceStatus := range o.ResourceStatuses {
		resourceStatuses = append(resourceStatuses, resourceStatus)
	}
	sort.Sort(resourceStatuses)

	return &Observation{
		LastEventType:    o.LastEventType,
		ResourceStatuses: resourceStatuses,
		Error:            o.Error,
	}
}

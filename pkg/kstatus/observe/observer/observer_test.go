// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package observer

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestStatusObserverRunner(t *testing.T) {
	testCases := map[string]struct {
		identifiers        []wait.ResourceIdentifier
		defaultObserver    ResourceObserver
		expectedEventTypes []event.EventType
	}{
		"no resources": {
			identifiers:        []wait.ResourceIdentifier{},
			expectedEventTypes: []event.EventType{event.CompletedEvent},
		},
		"single resource": {
			identifiers: []wait.ResourceIdentifier{
				{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Name:      "foo",
					Namespace: "bar",
				},
			},
			defaultObserver: &fakeObserver{
				resourceObservations: map[schema.GroupKind][]status.Status{
					schema.GroupKind{Group: "apps", Kind: "Deployment"}: { //nolint:gofmt
						status.InProgressStatus,
						status.CurrentStatus,
					},
				},
				resourceObservationCount: make(map[schema.GroupKind]int),
			},
			expectedEventTypes: []event.EventType{
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
				event.CompletedEvent,
			},
		},
		"multiple resources": {
			identifiers: []wait.ResourceIdentifier{
				{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Name:      "foo",
					Namespace: "default",
				},
				{
					GroupKind: schema.GroupKind{
						Group: "",
						Kind:  "Service",
					},
					Name:      "bar",
					Namespace: "default",
				},
			},
			defaultObserver: &fakeObserver{
				resourceObservations: map[schema.GroupKind][]status.Status{
					schema.GroupKind{Group: "apps", Kind: "Deployment"}: { //nolint:gofmt
						status.InProgressStatus,
						status.CurrentStatus,
					},
					schema.GroupKind{Group: "", Kind: "Service"}: { //nolint:gofmt
						status.InProgressStatus,
						status.InProgressStatus,
						status.CurrentStatus,
					},
				},
				resourceObservationCount: make(map[schema.GroupKind]int),
			},
			expectedEventTypes: []event.EventType{
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
				event.CompletedEvent,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ctx := context.Background()

			identifiers := tc.identifiers

			observer := Observer{
				AggregatorFactoryFunc: func(identifiers []wait.ResourceIdentifier) StatusAggregator {
					return newFakeAggregator(identifiers)
				},
				ReaderFactoryFunc: func(_ client.Reader, _ meta.RESTMapper, _ []wait.ResourceIdentifier) (
					ObserverReader, error) {
					return testutil.NewNoopObserverReader(), nil
				},
				ObserversFactoryFunc: func(_ ObserverReader, _ meta.RESTMapper) (
					resourceObservers map[schema.GroupKind]ResourceObserver, defaultObserver ResourceObserver) {
					return make(map[schema.GroupKind]ResourceObserver), tc.defaultObserver
				},
			}

			eventChannel := observer.Observe(ctx, identifiers, 2*time.Second, false)

			var eventTypes []event.EventType
			for ch := range eventChannel {
				eventTypes = append(eventTypes, ch.EventType)
			}

			assert.DeepEqual(t, tc.expectedEventTypes, eventTypes)
		})
	}
}

func TestNewStatusObserverRunnerCancellation(t *testing.T) {
	identifiers := make([]wait.ResourceIdentifier, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	timer := time.NewTimer(5 * time.Second)

	observer := Observer{
		AggregatorFactoryFunc: func(identifiers []wait.ResourceIdentifier) StatusAggregator {
			return newFakeAggregator(identifiers)
			//return aggregator.NewAllCurrentOrNotFoundStatusAggregator(identifiers)
		},
		ReaderFactoryFunc: func(_ client.Reader, _ meta.RESTMapper, _ []wait.ResourceIdentifier) (
			ObserverReader, error) {
			return testutil.NewNoopObserverReader(), nil
		},
		ObserversFactoryFunc: func(_ ObserverReader, _ meta.RESTMapper) (
			resourceObservers map[schema.GroupKind]ResourceObserver, defaultObserver ResourceObserver) {
			return make(map[schema.GroupKind]ResourceObserver), nil
		},
	}

	eventChannel := observer.Observe(ctx, identifiers, 2*time.Second, true)

	var lastEvent event.Event
	for {
		select {
		case e, more := <-eventChannel:
			timer.Stop()
			if more {
				lastEvent = e
			} else {
				if want, got := event.AbortedEvent, lastEvent.EventType; got != want {
					t.Errorf("Expected e to have type %s, but got %s", want, got)
				}
				return
			}
		case <-timer.C:
			t.Errorf("expected runner to time out, but it didn't")
			return
		}
	}
}

type fakeObserver struct {
	resourceObservations     map[schema.GroupKind][]status.Status
	resourceObservationCount map[schema.GroupKind]int
}

func (f *fakeObserver) Observe(_ context.Context, identifier wait.ResourceIdentifier) *event.ObservedResource {
	count := f.resourceObservationCount[identifier.GroupKind]
	observedResourceStatusSlice := f.resourceObservations[identifier.GroupKind]
	var observedResourceStatus status.Status
	if len(observedResourceStatusSlice) > count {
		observedResourceStatus = observedResourceStatusSlice[count]
	} else {
		observedResourceStatus = observedResourceStatusSlice[len(observedResourceStatusSlice)-1]
	}
	f.resourceObservationCount[identifier.GroupKind] = count + 1
	return &event.ObservedResource{
		Identifier: identifier,
		Status:     observedResourceStatus,
	}
}

func (f *fakeObserver) ObserveObject(_ context.Context, _ *unstructured.Unstructured) *event.ObservedResource {
	return nil
}

func (f *fakeObserver) SetComputeStatusFunc(_ ComputeStatusFunc) {}

func newFakeAggregator(identifiers []wait.ResourceIdentifier) *fakeAggregator {
	statuses := make(map[wait.ResourceIdentifier]status.Status)
	for _, id := range identifiers {
		statuses[id] = status.UnknownStatus
	}
	return &fakeAggregator{
		statuses: statuses,
	}
}

type fakeAggregator struct {
	statuses map[wait.ResourceIdentifier]status.Status
}

func (f *fakeAggregator) ResourceObserved(resource *event.ObservedResource) {
	f.statuses[resource.Identifier] = resource.Status
}

func (f *fakeAggregator) AggregateStatus() status.Status {
	for _, s := range f.statuses {
		if s != status.CurrentStatus {
			return status.InProgressStatus
		}
	}
	return status.CurrentStatus
}

func (f *fakeAggregator) Completed() bool {
	return f.AggregateStatus() == status.CurrentStatus
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"sort"
	"testing"
	"time"

	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

func TestCollectorStopsWhenEventChannelIsClosed(t *testing.T) {
	var identifiers []wait.ResourceIdentifier

	collector := NewResourceStatusCollector(identifiers)

	eventCh := make(chan event.Event)
	stopCh := make(chan struct{})
	defer close(stopCh)

	completedCh := collector.Listen(eventCh, stopCh)

	timer := time.NewTimer(3 * time.Second)

	close(eventCh)
	select {
	case <-timer.C:
		t.Errorf("expected collector to close the completedCh, but it didn't")
	case <-completedCh:
		timer.Stop()
	}
}

func TestCollectorStopWhenStopChannelIsClosed(t *testing.T) {
	var identifiers []wait.ResourceIdentifier

	collector := NewResourceStatusCollector(identifiers)

	eventCh := make(chan event.Event)
	defer close(eventCh)
	stopCh := make(chan struct{})

	completedCh := collector.Listen(eventCh, stopCh)

	timer := time.NewTimer(3 * time.Second)

	close(stopCh)
	select {
	case <-timer.C:
		t.Errorf("expected collector to close the completedCh, but it didn't")
	case <-completedCh:
		timer.Stop()
	}
}

var (
	deploymentGVK       = appsv1.SchemeGroupVersion.WithKind("Deployment")
	statefulSetGVK      = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
	resourceIdentifiers = map[string]wait.ResourceIdentifier{
		"deployment": {
			GroupKind: deploymentGVK.GroupKind(),
			Name:      "Foo",
			Namespace: "default",
		},
		"statefulSet": {
			GroupKind: statefulSetGVK.GroupKind(),
			Name:      "Bar",
			Namespace: "default",
		},
	}
)

func TestCollectorEventProcessing(t *testing.T) {
	testCases := map[string]struct {
		identifiers []wait.ResourceIdentifier
		events      []event.Event
	}{
		"no resources and no events": {},
		"single resource and single event": {
			identifiers: []wait.ResourceIdentifier{
				resourceIdentifiers["deployment"],
			},
			events: []event.Event{
				{
					EventType:       event.ResourceUpdateEvent,
					AggregateStatus: status.CurrentStatus,
					Resource: &event.ResourceStatus{
						Identifier: resourceIdentifiers["deployment"],
					},
				},
			},
		},
		"multiple resources and multiple events": {
			identifiers: []wait.ResourceIdentifier{
				resourceIdentifiers["deployment"],
				resourceIdentifiers["statefulSet"],
			},
			events: []event.Event{
				{
					EventType:       event.ResourceUpdateEvent,
					AggregateStatus: status.UnknownStatus,
					Resource: &event.ResourceStatus{
						Identifier: resourceIdentifiers["deployment"],
					},
				},
				{
					EventType:       event.ResourceUpdateEvent,
					AggregateStatus: status.InProgressStatus,
					Resource: &event.ResourceStatus{
						Identifier: resourceIdentifiers["statefulSet"],
					},
				},
				{
					EventType:       event.ResourceUpdateEvent,
					AggregateStatus: status.CurrentStatus,
					Resource: &event.ResourceStatus{
						Identifier: resourceIdentifiers["deployment"],
					},
				},
				{
					EventType:       event.ResourceUpdateEvent,
					AggregateStatus: status.InProgressStatus,
					Resource: &event.ResourceStatus{
						Identifier: resourceIdentifiers["statefulSet"],
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			collector := NewResourceStatusCollector(tc.identifiers)

			eventCh := make(chan event.Event)
			defer close(eventCh)
			stopCh := make(chan struct{})

			collector.Listen(eventCh, stopCh)

			var latestEvent *event.Event
			latestEventByIdentifier := make(map[wait.ResourceIdentifier]event.Event)
			for _, e := range tc.events {
				if e.Resource != nil {
					latestEventByIdentifier[e.Resource.Identifier] = e
				}
				ev := e
				latestEvent = &ev
				eventCh <- e
			}
			// Give the collector some time to process the event.
			<-time.NewTimer(time.Second).C

			observation := collector.LatestObservation()

			var expectedObservation *Observation
			if latestEvent != nil {
				expectedObservation = &Observation{
					LastEventType:   latestEvent.EventType,
					AggregateStatus: latestEvent.AggregateStatus,
				}
			} else {
				expectedObservation = &Observation{
					AggregateStatus: status.UnknownStatus,
				}
			}

			var resourceStatuses event.ResourceStatuses
			for _, e := range latestEventByIdentifier {
				resourceStatuses = append(resourceStatuses, e.Resource)
			}
			sort.Sort(resourceStatuses)
			expectedObservation.ResourceStatuses = resourceStatuses

			assert.DeepEqual(t, expectedObservation, observation)
		})
	}
}

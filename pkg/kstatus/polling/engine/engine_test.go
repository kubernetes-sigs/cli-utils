// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestStatusPollerRunner(t *testing.T) {
	testCases := map[string]struct {
		identifiers         []wait.ResourceIdentifier
		defaultStatusReader StatusReader
		expectedEventTypes  []event.EventType
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
			defaultStatusReader: &fakeStatusReader{
				resourceStatuses: map[schema.GroupKind][]status.Status{
					schema.GroupKind{Group: "apps", Kind: "Deployment"}: { //nolint:gofmt
						status.InProgressStatus,
						status.CurrentStatus,
					},
				},
				resourceStatusCount: make(map[schema.GroupKind]int),
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
			defaultStatusReader: &fakeStatusReader{
				resourceStatuses: map[schema.GroupKind][]status.Status{
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
				resourceStatusCount: make(map[schema.GroupKind]int),
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

			engine := PollerEngine{
				AggregatorFactoryFunc: func(identifiers []wait.ResourceIdentifier) StatusAggregator {
					return newFakeAggregator(identifiers)
				},
				ClusterReaderFactoryFunc: func(_ client.Reader, _ meta.RESTMapper, _ []wait.ResourceIdentifier) (
					ClusterReader, error) {
					return testutil.NewNoopClusterReader(), nil
				},
				StatusReadersFactoryFunc: func(_ ClusterReader, _ meta.RESTMapper) (
					statusReaders map[schema.GroupKind]StatusReader, defaultStatusReader StatusReader) {
					return make(map[schema.GroupKind]StatusReader), tc.defaultStatusReader
				},
			}

			eventChannel := engine.Poll(ctx, identifiers, 2*time.Second, false)

			var eventTypes []event.EventType
			for ch := range eventChannel {
				eventTypes = append(eventTypes, ch.EventType)
			}

			assert.DeepEqual(t, tc.expectedEventTypes, eventTypes)
		})
	}
}

func TestNewStatusPollerRunnerCancellation(t *testing.T) {
	identifiers := make([]wait.ResourceIdentifier, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	timer := time.NewTimer(5 * time.Second)

	engine := PollerEngine{
		AggregatorFactoryFunc: func(identifiers []wait.ResourceIdentifier) StatusAggregator {
			return newFakeAggregator(identifiers)
			//return aggregator.NewAllCurrentOrNotFoundStatusAggregator(identifiers)
		},
		ClusterReaderFactoryFunc: func(_ client.Reader, _ meta.RESTMapper, _ []wait.ResourceIdentifier) (
			ClusterReader, error) {
			return testutil.NewNoopClusterReader(), nil
		},
		StatusReadersFactoryFunc: func(_ ClusterReader, _ meta.RESTMapper) (
			statusReaders map[schema.GroupKind]StatusReader, defaultStatusReader StatusReader) {
			return make(map[schema.GroupKind]StatusReader), nil
		},
	}

	eventChannel := engine.Poll(ctx, identifiers, 2*time.Second, true)

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

type fakeStatusReader struct {
	resourceStatuses    map[schema.GroupKind][]status.Status
	resourceStatusCount map[schema.GroupKind]int
}

func (f *fakeStatusReader) ReadStatus(_ context.Context, identifier wait.ResourceIdentifier) *event.ResourceStatus {
	count := f.resourceStatusCount[identifier.GroupKind]
	resourceStatusSlice := f.resourceStatuses[identifier.GroupKind]
	var resourceStatus status.Status
	if len(resourceStatusSlice) > count {
		resourceStatus = resourceStatusSlice[count]
	} else {
		resourceStatus = resourceStatusSlice[len(resourceStatusSlice)-1]
	}
	f.resourceStatusCount[identifier.GroupKind] = count + 1
	return &event.ResourceStatus{
		Identifier: identifier,
		Status:     resourceStatus,
	}
}

func (f *fakeStatusReader) ReadStatusForObject(_ context.Context, _ *unstructured.Unstructured) *event.ResourceStatus {
	return nil
}

func (f *fakeStatusReader) SetComputeStatusFunc(_ ComputeStatusFunc) {}

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

func (f *fakeAggregator) ResourceStatus(resource *event.ResourceStatus) {
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

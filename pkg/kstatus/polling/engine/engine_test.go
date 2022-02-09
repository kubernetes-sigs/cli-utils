// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakecr "sigs.k8s.io/cli-utils/pkg/kstatus/polling/clusterreader/fake"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	fakemapper "sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestStatusPollerRunner(t *testing.T) {
	testCases := map[string]struct {
		identifiers         object.ObjMetadataSet
		defaultStatusReader StatusReader
		expectedEventTypes  []event.Type
	}{
		"single resource": {
			identifiers: object.ObjMetadataSet{
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
			expectedEventTypes: []event.Type{
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
			},
		},
		"multiple resources": {
			identifiers: object.ObjMetadataSet{
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
			expectedEventTypes: []event.Type{
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
				event.ResourceUpdateEvent,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			identifiers := tc.identifiers

			fakeMapper := fakemapper.NewFakeRESTMapper(
				appsv1.SchemeGroupVersion.WithKind("Deployment"),
				v1.SchemeGroupVersion.WithKind("Service"),
			)

			engine := PollerEngine{
				Mapper:              fakeMapper,
				DefaultStatusReader: tc.defaultStatusReader,
				StatusReaders:       []StatusReader{},
				ClusterReaderFactory: ClusterReaderFactoryFunc(func(client.Reader, meta.RESTMapper, object.ObjMetadataSet) (ClusterReader, error) {
					return fakecr.NewNoopClusterReader(), nil
				}),
			}

			options := Options{
				PollInterval: 2 * time.Second,
			}

			eventChannel := engine.Poll(ctx, identifiers, options)

			var eventTypes []event.Type
			for ch := range eventChannel {
				eventTypes = append(eventTypes, ch.Type)
				if len(eventTypes) == len(tc.expectedEventTypes) {
					cancel()
				}
			}

			assert.Equal(t, tc.expectedEventTypes, eventTypes)
		})
	}
}

func TestNewStatusPollerRunnerCancellation(t *testing.T) {
	identifiers := make(object.ObjMetadataSet, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	timer := time.NewTimer(5 * time.Second)

	engine := PollerEngine{
		ClusterReaderFactory: ClusterReaderFactoryFunc(func(client.Reader, meta.RESTMapper, object.ObjMetadataSet) (ClusterReader, error) {
			return fakecr.NewNoopClusterReader(), nil
		}),
	}

	options := Options{
		PollInterval: 2 * time.Second,
	}

	eventChannel := engine.Poll(ctx, identifiers, options)

	for {
		select {
		case <-eventChannel:
			timer.Stop()
			return
		case <-timer.C:
			t.Errorf("expected runner to time out, but it didn't")
			return
		}
	}
}

func TestNewStatusPollerRunnerCancellationWithMultipleResources(t *testing.T) {
	identifiers := object.ObjMetadataSet{
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
				Group: "apps",
				Kind:  "StatefulSet",
			},
			Name:      "bar",
			Namespace: "default",
		},
	}

	fakeMapper := fakemapper.NewFakeRESTMapper(
		appsv1.SchemeGroupVersion.WithKind("Deployment"),
		appsv1.SchemeGroupVersion.WithKind("StatefulSet"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	timer := time.NewTimer(10 * time.Second)

	engine := PollerEngine{
		Mapper: fakeMapper,
		ClusterReaderFactory: ClusterReaderFactoryFunc(func(client.Reader, meta.RESTMapper, object.ObjMetadataSet) (ClusterReader, error) {
			return &fakecr.FakeClusterReader{
				SyncErr: context.Canceled,
			}, nil
		}),
	}

	options := Options{
		PollInterval: 1 * time.Second,
	}

	eventChannel := engine.Poll(ctx, identifiers, options)

	var events []event.Event
loop:
	for {
		select {
		case e, ok := <-eventChannel:
			if !ok {
				timer.Stop()
				break loop
			}
			events = append(events, e)
		case <-timer.C:
			t.Errorf("expected runner to time out, but it didn't")
			return
		}
	}
	assert.Equal(t, 0, len(events))
}

func TestNewStatusPollerRunnerIdentifierValidation(t *testing.T) {
	identifiers := object.ObjMetadataSet{
		{
			GroupKind: schema.GroupKind{
				Group: "apps",
				Kind:  "Deployment",
			},
			Name: "foo",
		},
	}

	engine := PollerEngine{
		Mapper: fakemapper.NewFakeRESTMapper(
			appsv1.SchemeGroupVersion.WithKind("Deployment"),
		),
		ClusterReaderFactory: ClusterReaderFactoryFunc(func(client.Reader, meta.RESTMapper, object.ObjMetadataSet) (ClusterReader, error) {
			return fakecr.NewNoopClusterReader(), nil
		}),
	}

	eventChannel := engine.Poll(context.Background(), identifiers, Options{})

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case e := <-eventChannel:
		if e.Type != event.ErrorEvent {
			t.Errorf("expected an error event, but got %s", e.Type.String())
			return
		}
		err := e.Error
		if !strings.Contains(err.Error(), "namespace is not set") {
			t.Errorf("expected error with namespace not set, but got %v", err)
		}
		return
	case <-timer.C:
		t.Errorf("expected an error event, but didn't get one")
	}
}

type fakeStatusReader struct {
	resourceStatuses    map[schema.GroupKind][]status.Status
	resourceStatusCount map[schema.GroupKind]int
}

func (f *fakeStatusReader) Supports(schema.GroupKind) bool {
	return true
}

func (f *fakeStatusReader) ReadStatus(_ context.Context, _ ClusterReader, identifier object.ObjMetadata) (*event.ResourceStatus, error) {
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
	}, nil
}

func (f *fakeStatusReader) ReadStatusForObject(_ context.Context, _ ClusterReader, _ *unstructured.Unstructured) (*event.ResourceStatus, error) {
	return nil, nil
}

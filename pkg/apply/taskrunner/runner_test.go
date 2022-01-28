// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	depID = object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Namespace: "default",
		Name:      "dep",
	}
	cmID = object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "",
			Kind:  "ConfigMap",
		},
		Namespace: "default",
		Name:      "cm",
	}
)

func TestBaseRunner(t *testing.T) {
	testCases := map[string]struct {
		tasks              []Task
		statusEventsDelay  time.Duration
		statusEvents       []pollevent.Event
		expectedEventTypes []event.Type
		expectedWaitEvents []event.WaitEvent
	}{
		"wait task runs until condition is met": {
			tasks: []Task{
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 3 * time.Second,
				},
				NewWaitTask("wait", object.ObjMetadataSet{depID, cmID}, AllCurrent,
					1*time.Minute, testutil.NewFakeRESTMapper()),
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			statusEventsDelay: 5 * time.Second,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: cmID,
						Status:     status.CurrentStatus,
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depID,
						Status:     status.CurrentStatus,
					},
				},
			},
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.ApplyType,
				event.ActionGroupType,
				event.ActionGroupType,
				event.WaitType,   // deployment pending
				event.WaitType,   // configmap pending
				event.StatusType, // configmap current
				event.WaitType,   // configmap reconciled
				event.StatusType, // deployment current
				event.WaitType,   // deployment reconciled
				event.ActionGroupType,
				event.ActionGroupType,
				event.PruneType,
				event.ActionGroupType,
			},
			expectedWaitEvents: []event.WaitEvent{
				{
					GroupName:  "wait",
					Identifier: depID,
					Operation:  event.ReconcilePending,
				},
				{
					GroupName:  "wait",
					Identifier: cmID,
					Operation:  event.ReconcilePending,
				},
				{
					GroupName:  "wait",
					Identifier: cmID,
					Operation:  event.Reconciled,
				},
				{
					GroupName:  "wait",
					Identifier: depID,
					Operation:  event.Reconciled,
				},
			},
		},
		"wait task times out eventually (Unknown)": {
			tasks: []Task{
				NewWaitTask("wait", object.ObjMetadataSet{depID, cmID}, AllCurrent,
					2*time.Second, testutil.NewFakeRESTMapper()),
			},
			statusEventsDelay: time.Second,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: cmID,
						Status:     status.CurrentStatus,
					},
				},
			},
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.WaitType,   // configmap pending
				event.WaitType,   // deployment pending
				event.StatusType, // configmap current
				event.WaitType,   // configmap reconciled
				event.WaitType,   // deployment timeout error
				event.ActionGroupType,
			},
			expectedWaitEvents: []event.WaitEvent{
				{
					GroupName:  "wait",
					Identifier: depID,
					Operation:  event.ReconcilePending,
				},
				{
					GroupName:  "wait",
					Identifier: cmID,
					Operation:  event.ReconcilePending,
				},
				{
					GroupName:  "wait",
					Identifier: cmID,
					Operation:  event.Reconciled,
				},
				{
					GroupName:  "wait",
					Identifier: depID,
					Operation:  event.ReconcileTimeout,
				},
			},
		},
		"wait task times out eventually (InProgress)": {
			tasks: []Task{
				NewWaitTask("wait", object.ObjMetadataSet{depID, cmID}, AllCurrent,
					2*time.Second, testutil.NewFakeRESTMapper()),
			},
			statusEventsDelay: time.Second,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: cmID,
						Status:     status.CurrentStatus,
					},
				},
				{
					EventType: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depID,
						Status:     status.InProgressStatus,
					},
				},
			},
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.WaitType,   // configmap pending
				event.WaitType,   // deployment pending
				event.StatusType, // configmap current
				event.WaitType,   // configmap reconciled
				event.StatusType, // deployment inprogress
				event.WaitType,   // deployment timeout error
				event.ActionGroupType,
			},
			expectedWaitEvents: []event.WaitEvent{
				{
					GroupName:  "wait",
					Identifier: depID,
					Operation:  event.ReconcilePending,
				},
				{
					GroupName:  "wait",
					Identifier: cmID,
					Operation:  event.ReconcilePending,
				},
				{
					GroupName:  "wait",
					Identifier: cmID,
					Operation:  event.Reconciled,
				},
				{
					GroupName:  "wait",
					Identifier: depID,
					Operation:  event.ReconcileTimeout,
				},
			},
		},
		"tasks run in order": {
			tasks: []Task{
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 1 * time.Second,
				},
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 1 * time.Second,
				},
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 1 * time.Second,
				},
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 1 * time.Second,
				},
			},
			statusEventsDelay: 1 * time.Second,
			statusEvents:      []pollevent.Event{},
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.ApplyType,
				event.ActionGroupType,
				event.ActionGroupType,
				event.PruneType,
				event.ActionGroupType,
				event.ActionGroupType,
				event.ApplyType,
				event.ActionGroupType,
				event.ActionGroupType,
				event.PruneType,
				event.ActionGroupType,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			taskQueue := make(chan Task, len(tc.tasks))
			for _, tsk := range tc.tasks {
				taskQueue <- tsk
			}

			ids := object.ObjMetadataSet{} // unused by fake poller
			poller := newFakePoller(tc.statusEvents)
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := NewTaskContext(eventChannel, resourceCache)
			runner := NewTaskStatusRunner(ids, poller)

			// Use a WaitGroup to make sure changes in the goroutines
			// are visible to the main goroutine.
			var wg sync.WaitGroup

			statusChannel := make(chan pollevent.Event)
			wg.Add(1)
			go func() {
				defer wg.Done()

				time.Sleep(tc.statusEventsDelay)
				poller.Start()
			}()

			var events []event.Event
			wg.Add(1)
			go func() {
				defer wg.Done()

				for msg := range eventChannel {
					events = append(events, msg)
				}
			}()

			opts := Options{EmitStatusEvents: true}
			ctx := context.Background()
			err := runner.Run(ctx, taskContext, taskQueue, opts)
			close(statusChannel)
			close(eventChannel)
			wg.Wait()

			assert.NoError(t, err)

			if want, got := len(tc.expectedEventTypes), len(events); want != got {
				t.Errorf("expected %d events, but got %d", want, got)
			}
			var waitEvents []event.WaitEvent
			for i, e := range events {
				expectedEventType := tc.expectedEventTypes[i]
				if want, got := expectedEventType, e.Type; want != got {
					t.Errorf("expected event type %s, but got %s",
						want, got)
				}
				if e.Type == event.WaitType {
					waitEvents = append(waitEvents, e.WaitEvent)
				}
			}
			assert.Equal(t, tc.expectedWaitEvents, waitEvents)
		})
	}
}

func TestBaseRunnerCancellation(t *testing.T) {
	testError := fmt.Errorf("this is a test error")

	testCases := map[string]struct {
		tasks              []Task
		statusEventsDelay  time.Duration
		statusEvents       []pollevent.Event
		contextTimeout     time.Duration
		expectedError      error
		expectedEventTypes []event.Type
	}{
		"cancellation while custom task is running": {
			tasks: []Task{
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 4 * time.Second,
				},
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextTimeout: 2 * time.Second,
			expectedError:  context.DeadlineExceeded,
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.ApplyType,
				event.ActionGroupType,
			},
		},
		"cancellation while wait task is running": {
			tasks: []Task{
				NewWaitTask("wait", object.ObjMetadataSet{depID}, AllCurrent,
					20*time.Second, testutil.NewFakeRESTMapper()),
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextTimeout: 2 * time.Second,
			expectedError:  context.DeadlineExceeded,
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.WaitType, // pending
				event.ActionGroupType,
			},
		},
		"error while custom task is running": {
			tasks: []Task{
				&fakeApplyTask{
					name: "apply-0",
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 2 * time.Second,
					err:      testError,
				},
				&fakeApplyTask{
					name: "prune-0",
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextTimeout: 30 * time.Second,
			expectedError:  fmt.Errorf(`task failed (action: "Apply", name: "apply-0"): %w`, testError),
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.ApplyType,
				event.ActionGroupType,
			},
		},
		"error from status poller while wait task is running": {
			tasks: []Task{
				NewWaitTask("wait", object.ObjMetadataSet{depID}, AllCurrent,
					20*time.Second, testutil.NewFakeRESTMapper()),
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			statusEventsDelay: 2 * time.Second,
			statusEvents: []pollevent.Event{
				{
					EventType: pollevent.ErrorEvent,
					Error:     testError,
				},
			},
			contextTimeout: 30 * time.Second,
			expectedError:  fmt.Errorf("polling for status failed: %w", testError),
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.WaitType, // pending
				event.ActionGroupType,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			taskQueue := make(chan Task, len(tc.tasks))
			for _, tsk := range tc.tasks {
				taskQueue <- tsk
			}

			ids := object.ObjMetadataSet{} // unused by fake poller
			poller := newFakePoller(tc.statusEvents)
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := NewTaskContext(eventChannel, resourceCache)
			runner := NewTaskStatusRunner(ids, poller)

			// Use a WaitGroup to make sure changes in the goroutines
			// are visible to the main goroutine.
			var wg sync.WaitGroup

			statusChannel := make(chan pollevent.Event)
			wg.Add(1)
			go func() {
				defer wg.Done()

				time.Sleep(tc.statusEventsDelay)
				poller.Start()
			}()

			var events []event.Event
			wg.Add(1)
			go func() {
				defer wg.Done()

				for msg := range eventChannel {
					events = append(events, msg)
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), tc.contextTimeout)
			defer cancel()

			opts := Options{EmitStatusEvents: true}
			err := runner.Run(ctx, taskContext, taskQueue, opts)
			close(statusChannel)
			close(eventChannel)
			wg.Wait()

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			if want, got := len(tc.expectedEventTypes), len(events); want != got {
				t.Errorf("expected %d events, but got %d", want, got)
			}
			for i, e := range events {
				expectedEventType := tc.expectedEventTypes[i]
				if want, got := expectedEventType, e.Type; want != got {
					t.Errorf("expected event type %s, but got %s",
						want, got)
				}
			}
		})
	}
}

type fakeApplyTask struct {
	name        string
	resultEvent event.Event
	duration    time.Duration
	err         error
}

func (f *fakeApplyTask) Name() string {
	return f.name
}

func (f *fakeApplyTask) Action() event.ResourceAction {
	return event.ApplyAction
}

func (f *fakeApplyTask) Identifiers() object.ObjMetadataSet {
	return object.ObjMetadataSet{}
}

func (f *fakeApplyTask) Start(taskContext *TaskContext) {
	go func() {
		<-time.NewTimer(f.duration).C
		taskContext.SendEvent(f.resultEvent)
		taskContext.TaskChannel() <- TaskResult{
			Err: f.err,
		}
	}()
}

func (f *fakeApplyTask) Cancel(_ *TaskContext) {}

func (f *fakeApplyTask) StatusUpdate(_ *TaskContext, _ object.ObjMetadata) {}

type fakePoller struct {
	start  chan struct{}
	events []pollevent.Event
}

func newFakePoller(statusEvents []pollevent.Event) *fakePoller {
	return &fakePoller{
		events: statusEvents,
		start:  make(chan struct{}),
	}
}

// Start events being sent on the status channel
func (f *fakePoller) Start() {
	close(f.start)
}

func (f *fakePoller) Poll(ctx context.Context, _ object.ObjMetadataSet, _ polling.PollOptions) <-chan pollevent.Event {
	eventChannel := make(chan pollevent.Event)
	go func() {
		defer close(eventChannel)
		// wait until started to send the events
		<-f.start
		for _, f := range f.events {
			eventChannel <- f
		}
		// wait until cancelled to close the event channel and exit
		<-ctx.Done()
	}()
	return eventChannel
}

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
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
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
		tasks                     []Task
		statusEventsDelay         time.Duration
		statusEvents              []pollevent.Event
		expectedEventTypes        []event.Type
		expectedError             error
		expectedTimedOutResources []TimedOutResource
	}{
		"wait task runs until condition is met": {
			tasks: []Task{
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 3 * time.Second,
				},
				NewWaitTask("wait", []object.ObjMetadata{depID, cmID}, AllCurrent,
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
				event.StatusType,
				event.StatusType,
				event.ActionGroupType,
				event.ActionGroupType,
				event.PruneType,
				event.ActionGroupType,
			},
		},
		"wait task times out eventually": {
			tasks: []Task{
				NewWaitTask("wait", []object.ObjMetadata{depID, cmID}, AllCurrent,
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
				event.StatusType,
			},
			expectedError: &TimeoutError{},
			expectedTimedOutResources: []TimedOutResource{
				{
					Identifier: depID,
					Status:     status.UnknownStatus,
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
			runner := newBaseRunner()
			eventChannel := make(chan event.Event)
			taskQueue := make(chan Task, len(tc.tasks))
			for _, tsk := range tc.tasks {
				taskQueue <- tsk
			}

			// Use a WaitGroup to make sure changes in the goroutines
			// are visible to the main goroutine.
			var wg sync.WaitGroup

			statusChannel := make(chan pollevent.Event)
			wg.Add(1)
			go func() {
				defer wg.Done()

				<-time.NewTimer(tc.statusEventsDelay).C
				for _, se := range tc.statusEvents {
					statusChannel <- se
				}
			}()

			var events []event.Event
			wg.Add(1)
			go func() {
				defer wg.Done()

				for msg := range eventChannel {
					events = append(events, msg)
				}
			}()

			err := runner.run(context.Background(), taskQueue, statusChannel,
				eventChannel, baseOptions{emitStatusEvents: true})
			close(statusChannel)
			close(eventChannel)
			wg.Wait()

			if tc.expectedError != nil {
				assert.IsType(t, tc.expectedError, err)
				if timeoutError, ok := err.(*TimeoutError); ok {
					assert.ElementsMatch(t, tc.expectedTimedOutResources,
						timeoutError.TimedOutResources)
				}
				return
			} else if err != nil {
				t.Errorf("expected no error, but got %v", err)
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

func TestBaseRunnerCancellation(t *testing.T) {
	testError := fmt.Errorf("this is a test error")

	testCases := map[string]struct {
		tasks              []Task
		statusEventsDelay  time.Duration
		statusEvents       []pollevent.Event
		contextTimeout     time.Duration
		contextCancel      time.Duration
		expectedError      error
		expectedEventTypes []event.Type
	}{
		"timeout while custom task is running": {
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
		"timeout while wait task is running": {
			tasks: []Task{
				NewWaitTask(
					"wait",
					[]object.ObjMetadata{depID},
					AllCurrent,
					20*time.Second,
					testutil.NewFakeRESTMapper(),
				),
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
				event.ActionGroupType,
			},
		},
		"cancel while wait task is running": {
			tasks: []Task{
				NewWaitTask(
					"wait",
					[]object.ObjMetadata{depID},
					AllCurrent,
					20*time.Second,
					testutil.NewFakeRESTMapper(),
				),
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextCancel: 2 * time.Second,
			expectedError: context.Canceled,
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.ActionGroupType,
			},
		},
		"error while custom task is running": {
			tasks: []Task{
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 2 * time.Second,
					err:      testError,
				},
				&fakeApplyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextTimeout: 30 * time.Second,
			expectedError:  testError,
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.ApplyType,
				event.ActionGroupType,
			},
		},
		"error from status poller while wait task is running": {
			tasks: []Task{
				NewWaitTask("wait", []object.ObjMetadata{depID}, AllCurrent,
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
			expectedError:  context.Canceled,
			expectedEventTypes: []event.Type{
				event.ActionGroupType,
				event.ActionGroupType,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			runner := newBaseRunner()
			eventChannel := make(chan event.Event)

			taskQueue := make(chan Task, len(tc.tasks))
			for _, tsk := range tc.tasks {
				taskQueue <- tsk
			}

			// Use a WaitGroup to make sure changes in the goroutines
			// are visible to the main goroutine.
			var wg sync.WaitGroup

			statusChannel := make(chan pollevent.Event)
			wg.Add(1)
			go func() {
				defer wg.Done()

				<-time.NewTimer(tc.statusEventsDelay).C
				for _, se := range tc.statusEvents {
					statusChannel <- se
				}
			}()

			var events []event.Event
			wg.Add(1)
			go func() {
				defer wg.Done()

				for msg := range eventChannel {
					events = append(events, msg)
				}
			}()

			ctx := context.Background()
			var cancel context.CancelFunc
			if tc.contextTimeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, tc.contextTimeout)
				defer cancel()
			} else if tc.contextCancel > 0 {
				ctx, cancel = context.WithCancel(ctx)
				go func() {
					time.Sleep(tc.contextCancel)
					cancel()
				}()
				defer cancel()
			}

			err := runner.run(
				ctx,
				taskQueue,
				statusChannel,
				eventChannel,
				baseOptions{emitStatusEvents: false},
			)
			close(statusChannel)
			close(eventChannel)
			wg.Wait()

			if tc.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError, err)
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

func (f *fakeApplyTask) Identifiers() []object.ObjMetadata {
	return []object.ObjMetadata{}
}

func (f *fakeApplyTask) Start(taskContext *TaskContext) {
	go func() {
		var err error
		ctx := taskContext.Context()
		timer := time.NewTimer(f.duration)
		select {
		case <-ctx.Done():
			// context cancel/timeout
			err = ctx.Err()
		case <-timer.C:
			// task duration timeout
			err = f.err
		}
		taskContext.EventChannel() <- f.resultEvent
		taskContext.TaskChannel() <- TaskResult{Err: err}
	}()
}

func (f *fakeApplyTask) OnStatusEvent(taskContext *TaskContext, e event.StatusEvent) {}

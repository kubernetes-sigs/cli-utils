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
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
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
		identifiers               []object.ObjMetadata
		tasks                     []Task
		statusEventsDelay         time.Duration
		statusEvents              []pollevent.Event
		expectedEventTypes        []event.Type
		expectedError             error
		expectedTimedOutResources []TimedOutResource
	}{
		"wait task runs until condition is met": {
			identifiers: []object.ObjMetadata{depID, cmID},
			tasks: []Task{
				&busyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 3 * time.Second,
				},
				NewWaitTask([]object.ObjMetadata{depID, cmID}, AllCurrent,
					1*time.Minute),
				&busyTask{
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
				event.ApplyType,
				event.StatusType,
				event.StatusType,
				event.PruneType,
			},
		},
		"wait task times out eventually": {
			identifiers: []object.ObjMetadata{depID, cmID},
			tasks: []Task{
				NewWaitTask([]object.ObjMetadata{depID, cmID}, AllCurrent,
					2*time.Second),
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
			identifiers: []object.ObjMetadata{},
			tasks: []Task{
				&busyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 1 * time.Second,
				},
				&busyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 1 * time.Second,
				},
				&busyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 1 * time.Second,
				},
				&busyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 1 * time.Second,
				},
			},
			statusEventsDelay: 1 * time.Second,
			statusEvents:      []pollevent.Event{},
			expectedEventTypes: []event.Type{
				event.ApplyType,
				event.PruneType,
				event.ApplyType,
				event.PruneType,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			runner := newBaseRunner(newResourceStatusCollector(tc.identifiers))
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
		identifiers        []object.ObjMetadata
		tasks              []Task
		statusEventsDelay  time.Duration
		statusEvents       []pollevent.Event
		contextTimeout     time.Duration
		expectedError      error
		expectedEventTypes []event.Type
	}{
		"cancellation while custom task is running": {
			identifiers: []object.ObjMetadata{depID},
			tasks: []Task{
				&busyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 4 * time.Second,
				},
				&busyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextTimeout: 2 * time.Second,
			expectedEventTypes: []event.Type{
				event.ApplyType,
			},
		},
		"cancellation while wait task is running": {
			identifiers: []object.ObjMetadata{depID},
			tasks: []Task{
				NewWaitTask([]object.ObjMetadata{depID}, AllCurrent, 20*time.Second),
				&busyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextTimeout:     2 * time.Second,
			expectedEventTypes: []event.Type{},
		},
		"error while custom task is running": {
			identifiers: []object.ObjMetadata{depID},
			tasks: []Task{
				&busyTask{
					resultEvent: event.Event{
						Type: event.ApplyType,
					},
					duration: 2 * time.Second,
					err:      testError,
				},
				&busyTask{
					resultEvent: event.Event{
						Type: event.PruneType,
					},
					duration: 2 * time.Second,
				},
			},
			contextTimeout: 30 * time.Second,
			expectedError:  testError,
			expectedEventTypes: []event.Type{
				event.ApplyType,
			},
		},
		"error from status poller while wait task is running": {
			identifiers: []object.ObjMetadata{depID},
			tasks: []Task{
				NewWaitTask([]object.ObjMetadata{depID}, AllCurrent, 20*time.Second),
				&busyTask{
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
			contextTimeout:     30 * time.Second,
			expectedError:      testError,
			expectedEventTypes: []event.Type{},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			runner := newBaseRunner(newResourceStatusCollector(tc.identifiers))
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

			ctx, cancel := context.WithTimeout(context.Background(), tc.contextTimeout)
			defer cancel()
			err := runner.run(ctx, taskQueue, statusChannel, eventChannel,
				baseOptions{emitStatusEvents: false})
			close(statusChannel)
			close(eventChannel)
			wg.Wait()

			if tc.expectedError == nil && err != nil {
				t.Errorf("expected no error, but got %v", err)
			}

			if tc.expectedError != nil && err == nil {
				t.Errorf("expected error %v, but didn't get one", tc.expectedError)
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

type busyTask struct {
	resultEvent event.Event
	duration    time.Duration
	err         error
}

func (b *busyTask) Start(taskContext *TaskContext) {
	go func() {
		<-time.NewTimer(b.duration).C
		taskContext.EventChannel() <- b.resultEvent
		taskContext.TaskChannel() <- TaskResult{
			Err: b.err,
		}
	}()
}

func (b *busyTask) ClearTimeout() {}

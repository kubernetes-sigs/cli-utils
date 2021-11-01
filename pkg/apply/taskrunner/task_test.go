// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var testDeploymentYAML = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo
  namespace: default
  uid: dep-uid
  generation: 1
spec:
  replicas: 1
`

func TestWaitTask_TimeoutTriggered(t *testing.T) {
	testDeploymentID := testutil.ToIdentifier(t, testDeploymentYAML)
	taskName := "wait"
	ids := object.ObjMetadataSet{
		testDeploymentID,
	}
	waitTimeout := 2 * time.Second
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	task.Start(taskContext)

	receivedTaskResults, receivedEvents, err := consumeUntilTimeout(taskContext, 5*time.Second)

	expectedEvents := []event.Event{
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName: taskName,
				Error: &TimeoutError{
					Identifiers: ids,
					Timeout:     waitTimeout,
					Condition:   AllCurrent,
					TimedOutResources: []TimedOutResource{
						{
							Identifier: testDeploymentID,
							Status:     status.UnknownStatus,
							Message:    "resource not cached",
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expectedEvents, receivedEvents)

	expectedTaskResults := []TaskResult{
		{}, // empty == completed/cancelled
	}
	testutil.AssertEqual(t, expectedTaskResults, receivedTaskResults)

	// Expect timeout, because channels are not closed by the WaitTask
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestWaitTask_TimeoutCancelled(t *testing.T) {
	testDeploymentID := testutil.ToIdentifier(t, testDeploymentYAML)
	taskName := "wait"
	ids := object.ObjMetadataSet{
		testDeploymentID,
	}
	waitTimeout := 2 * time.Second
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	task.Start(taskContext)
	task.ClearTimeout()

	receivedTaskResults, receivedEvents, err := consumeUntilTimeout(taskContext, 5*time.Second)

	expectedEvents := []event.Event{}
	testutil.AssertEqual(t, expectedEvents, receivedEvents)

	expectedTaskResults := []TaskResult{}
	testutil.AssertEqual(t, expectedTaskResults, receivedTaskResults)

	// Expect timeout, because channels are not closed by the WaitTask
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestWaitTask_SingleTaskResult(t *testing.T) {
	task := NewWaitTask("wait", object.ObjMetadataSet{}, AllCurrent,
		2*time.Second, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	task.Start(taskContext)

	var completeWg sync.WaitGroup
	for i := 0; i < 10; i++ {
		completeWg.Add(1)
		go func() {
			defer completeWg.Done()
			task.complete(taskContext)
		}()
	}
	completeWg.Wait()

	receivedTaskResults, receivedEvents, err := consumeUntilTimeout(taskContext, 5*time.Second)

	expectedEvents := []event.Event{}
	testutil.AssertEqual(t, expectedEvents, receivedEvents)

	expectedTaskResults := []TaskResult{
		{}, // empty == completed/cancelled
	}
	testutil.AssertEqual(t, expectedTaskResults, receivedTaskResults)

	// Expect timeout, because channels are not closed by the WaitTask
	assert.Equal(t, context.DeadlineExceeded, err)
}

func consumeUntilTimeout(taskContext *TaskContext, timeout time.Duration) ([]TaskResult, []event.Event, error) {
	taskResults := []TaskResult{}
	taskChClosed := false
	events := []event.Event{}
	eventChClosed := false
	timer := time.NewTimer(timeout)
	for {
		select {
		case tr, ok := <-taskContext.TaskChannel():
			if !ok {
				taskChClosed = true
				if eventChClosed {
					timer.Stop()
					return taskResults, events, nil
				}
			}
			taskResults = append(taskResults, tr)
		case e, ok := <-taskContext.EventChannel():
			if !ok {
				eventChClosed = true
				if taskChClosed {
					timer.Stop()
					return taskResults, events, nil
				}
			}
			events = append(events, e)
		case <-timer.C:
			// timed out
			return taskResults, events, context.DeadlineExceeded
		}
	}
}

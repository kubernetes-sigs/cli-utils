// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var testDeployment1YAML = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: a
  namespace: default
  uid: dep-uid-a
  generation: 1
spec:
  replicas: 1
`

var testDeployment2YAML = `
apiVersion: v1
kind: Deployment
metadata:
  name: b
  namespace: default
  uid: dep-uid-b
  generation: 1
spec:
  replicas: 2
`

var testDeployment3YAML = `
apiVersion: v1
kind: Deployment
metadata:
  name: c
  namespace: default
  uid: dep-uid-c
  generation: 1
spec:
  replicas: 3
`

var testDeployment4YAML = `
apiVersion: v1
kind: Deployment
metadata:
  name: d
  namespace: default
  uid: dep-uid-d
  generation: 1
spec:
  replicas: 4
`

func TestWaitTask_CompleteEventually(t *testing.T) {
	testDeployment1ID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment1 := testutil.Unstructured(t, testDeployment1YAML)
	testDeployment2ID := testutil.ToIdentifier(t, testDeployment2YAML)
	testDeployment2 := testutil.Unstructured(t, testDeployment2YAML)
	testDeployment3ID := testutil.ToIdentifier(t, testDeployment3YAML)
	testDeployment4ID := testutil.ToIdentifier(t, testDeployment4YAML)
	ids := object.ObjMetadataSet{
		testDeployment1ID,
		testDeployment2ID,
		testDeployment3ID,
		testDeployment4ID,
	}
	waitTimeout := 2 * time.Second
	taskName := "wait-1"
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	// mark deployment 1 & 2 as applied
	taskContext.AddSuccessfulApply(testDeployment1ID, "unused", 1)
	taskContext.AddSuccessfulApply(testDeployment2ID, "unused", 1)

	// mark deployment 3 as failed
	taskContext.AddFailedApply(testDeployment3ID)

	// mark deployment 4 as skipped
	taskContext.AddSkippedApply(testDeployment4ID)

	// run task async, to let the test collect events
	go func() {
		// start the task
		task.Start(taskContext)

		// mark deployment1 as Current
		resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
			Resource: testDeployment1,
			Status:   status.CurrentStatus,
		})
		// tell the WaitTask deployment1 has new status
		task.StatusUpdate(taskContext, testDeployment1ID)

		// mark deployment2 as InProgress
		resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
			Resource: testDeployment2,
			Status:   status.InProgressStatus,
		})
		// tell the WaitTask deployment2 has new status
		task.StatusUpdate(taskContext, testDeployment2ID)

		// mark deployment2 as Current
		resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
			Resource: testDeployment2,
			Status:   status.CurrentStatus,
		})
		// tell the WaitTask deployment2 has new status
		task.StatusUpdate(taskContext, testDeployment2ID)
	}()

	// wait for task result
	timer := time.NewTimer(5 * time.Second)
	receivedEvents := []event.Event{}
loop:
	for {
		select {
		case e := <-taskContext.EventChannel():
			receivedEvents = append(receivedEvents, e)
		case res := <-taskContext.TaskChannel():
			timer.Stop()
			assert.NoError(t, res.Err)
			break loop
		case <-timer.C:
			t.Fatalf("timed out waiting for TaskResult")
		}
	}

	// Expect an event for every object (sorted).
	expectedEvents := []event.Event{
		// skipped/reconciled/pending events first, in the order provided to the WaitTask
		// deployment1 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment1ID,
				Operation:  event.ReconcilePending,
			},
		},
		// deployment2 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Operation:  event.ReconcilePending,
			},
		},
		// deployment3 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment3ID,
				Operation:  event.ReconcileSkipped,
			},
		},
		// deployment4 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment4ID,
				Operation:  event.ReconcileSkipped,
			},
		},
		// current events next, in the order of status updates
		// deployment1 current
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment1ID,
				Operation:  event.Reconciled,
			},
		},
		// deployment2 current
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Operation:  event.Reconciled,
			},
		},
	}
	testutil.AssertEqual(t, receivedEvents, expectedEvents)
}

func TestWaitTask_Timeout(t *testing.T) {
	testDeployment1ID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment1 := testutil.Unstructured(t, testDeployment1YAML)
	testDeployment2ID := testutil.ToIdentifier(t, testDeployment2YAML)
	testDeployment3ID := testutil.ToIdentifier(t, testDeployment3YAML)
	testDeployment4ID := testutil.ToIdentifier(t, testDeployment4YAML)
	ids := object.ObjMetadataSet{
		testDeployment1ID,
		testDeployment2ID,
		testDeployment3ID,
		testDeployment4ID,
	}
	waitTimeout := 2 * time.Second
	taskName := "wait-2"
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	// mark deployment 1 & 2 as applied
	taskContext.AddSuccessfulApply(testDeployment1ID, "unused", 1)
	taskContext.AddSuccessfulApply(testDeployment2ID, "unused", 1)

	// mark deployment 3 as failed
	taskContext.AddFailedApply(testDeployment3ID)

	// mark deployment 4 as skipped
	taskContext.AddSkippedApply(testDeployment4ID)

	// run task async, to let the test collect events
	go func() {
		// start the task
		task.Start(taskContext)
		// mark deployment1 as Current
		resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
			Resource: testDeployment1,
			Status:   status.CurrentStatus,
		})
		// tell the WaitTask deployment1 has new status
		task.StatusUpdate(taskContext, testDeployment1ID)
	}()

	// wait for task result
	timer := time.NewTimer(5 * time.Second)
	receivedEvents := []event.Event{}
loop:
	for {
		select {
		case e := <-taskContext.EventChannel():
			receivedEvents = append(receivedEvents, e)
		case res := <-taskContext.TaskChannel():
			timer.Stop()
			assert.NoError(t, res.Err)
			break loop
		case <-timer.C:
			t.Fatalf("timed out waiting for TaskResult")
		}
	}

	// Expect an event for every object (sorted).
	expectedEvents := []event.Event{
		// skipped/reconciled/pending events first, in the order provided to the WaitTask
		// deployment1 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment1ID,
				Operation:  event.ReconcilePending,
			},
		},
		// deployment2 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Operation:  event.ReconcilePending,
			},
		},
		// deployment3 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment3ID,
				Operation:  event.ReconcileSkipped,
			},
		},
		// deployment4 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment4ID,
				Operation:  event.ReconcileSkipped,
			},
		},
		// current events next, in the order of status updates
		// deployment1 current
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment1ID,
				Operation:  event.Reconciled,
			},
		},
		// timeout events last, in the order provided to the WaitTask
		// deployment2 timeout
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Operation:  event.ReconcileTimeout,
			},
		},
	}
	testutil.AssertEqual(t, receivedEvents, expectedEvents)
}

func TestWaitTask_StartAndComplete(t *testing.T) {
	testDeploymentID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment := testutil.Unstructured(t, testDeployment1YAML)
	ids := object.ObjMetadataSet{
		testDeploymentID,
	}
	waitTimeout := 2 * time.Second
	taskName := "wait-3"
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	// mark the deployment as applied
	appliedGeneration := int64(1)
	taskContext.AddSuccessfulApply(testDeploymentID, "unused", appliedGeneration)

	// mark the deployment as Current before starting
	resourceCache.Put(testDeploymentID, cache.ResourceStatus{
		Resource: testDeployment,
		Status:   status.CurrentStatus,
	})

	// run task async, to let the test collect events
	go func() {
		// start the task
		task.Start(taskContext)
	}()

	// wait for first task result
	timer := time.NewTimer(5 * time.Second)
	receivedEvents := []event.Event{}
loop:
	for {
		select {
		case e := <-taskContext.EventChannel():
			receivedEvents = append(receivedEvents, e)
		case res := <-taskContext.TaskChannel():
			timer.Stop()
			assert.NoError(t, res.Err)
			break loop
		case <-timer.C:
			t.Fatalf("timed out waiting for TaskResult")
		}
	}

	expectedEvents := []event.Event{
		// deployment1 current (no pending event when Current before start)
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeploymentID,
				Operation:  event.Reconciled,
			},
		},
	}
	testutil.AssertEqual(t, receivedEvents, expectedEvents)
}

func TestWaitTask_Cancel(t *testing.T) {
	testDeploymentID := testutil.ToIdentifier(t, testDeployment1YAML)
	ids := object.ObjMetadataSet{
		testDeploymentID,
	}
	waitTimeout := 5 * time.Second
	taskName := "wait-4"
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	// run task async, to let the test collect events
	go func() {
		// start the task
		task.Start(taskContext)

		// wait a bit
		time.Sleep(1 * time.Second)

		// cancel immediately (simulate context cancel from baseRunner)
		task.Cancel(taskContext)
	}()

	// wait for first task result
	timer := time.NewTimer(10 * time.Second)
	receivedEvents := []event.Event{}
loop:
	for {
		select {
		case e := <-taskContext.EventChannel():
			receivedEvents = append(receivedEvents, e)
		case res := <-taskContext.TaskChannel():
			timer.Stop()
			assert.NoError(t, res.Err)
			break loop
		case <-timer.C:
			t.Fatalf("timed out waiting for TaskResult")
		}
	}

	// no timeout events sent on cancel
	expectedEvents := []event.Event{
		// skipped/reconciled/pending events first, in the order provided to the WaitTask
		// deployment1 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeploymentID,
				Operation:  event.ReconcilePending,
			},
		},
	}
	testutil.AssertEqual(t, receivedEvents, expectedEvents)
}

func TestWaitTask_SingleTaskResult(t *testing.T) {
	testDeploymentID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment := testutil.Unstructured(t, testDeployment1YAML)
	ids := object.ObjMetadataSet{
		testDeploymentID,
	}
	waitTimeout := 3 * time.Second
	taskName := "wait-5"
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	// buffer events, because they're sent by StatusUpdate
	eventChannel := make(chan event.Event, 10)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(eventChannel, resourceCache)
	defer close(eventChannel)

	// mark the deployment as applied
	appliedGeneration := int64(1)
	taskContext.AddSuccessfulApply(testDeploymentID, "unused", appliedGeneration)

	// run task async, to let the test collect events
	go func() {
		// start the task
		task.Start(taskContext)

		// wait a bit
		time.Sleep(1 * time.Second)

		// mark the deployment as Current
		resourceCache.Put(testDeploymentID, cache.ResourceStatus{
			Resource: withGeneration(testDeployment, appliedGeneration),
			Status:   status.CurrentStatus,
		})

		// send multiple status updates
		for i := 0; i < 10; i++ {
			task.StatusUpdate(taskContext, testDeploymentID)
		}
	}()

	// wait for timeout
	timer := time.NewTimer(5 * time.Second)
	receivedEvents := []event.Event{}
	receivedResults := []TaskResult{}
loop:
	for {
		select {
		case e := <-taskContext.EventChannel():
			receivedEvents = append(receivedEvents, e)
		case res := <-taskContext.TaskChannel():
			receivedResults = append(receivedResults, res)
		case <-timer.C:
			break loop
		}
	}

	expectedEvents := []event.Event{
		// skipped/reconciled/pending events first, in the order provided to the WaitTask
		// deployment1 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeploymentID,
				Operation:  event.ReconcilePending,
			},
		},
		// deployment1 reconciled
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeploymentID,
				Operation:  event.Reconciled,
			},
		},
	}
	testutil.AssertEqual(t, receivedEvents, expectedEvents)

	expectedResults := []TaskResult{
		{}, // Empty result means success
	}
	assert.Equal(t, expectedResults, receivedResults)
}

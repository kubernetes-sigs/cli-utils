// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
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
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID, "unused", 1)
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID, "unused", 1)

	// mark deployment 3 as failed
	taskContext.InventoryManager().AddFailedApply(testDeployment3ID)

	// mark deployment 4 as skipped
	taskContext.InventoryManager().AddSkippedApply(testDeployment4ID)

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
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.Inventory{
		Status: inventory.InventoryStatus{
			Objects: []inventory.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSucceeded,
					Reconcile:       inventory.ReconcileSucceeded,
					UID:             "unused",
					Generation:      1,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSucceeded,
					Reconcile:       inventory.ReconcileSucceeded,
					UID:             "unused",
					Generation:      1,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment3ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationFailed,
					Reconcile:       inventory.ReconcileSkipped,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment4ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSkipped,
					Reconcile:       inventory.ReconcileSkipped,
				},
			},
		},
	}
	testutil.AssertEqual(t, &expectedInventory, taskContext.InventoryManager().Inventory())
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
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID, "unused", 1)
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID, "unused", 1)

	// mark deployment 3 as failed
	taskContext.InventoryManager().AddFailedApply(testDeployment3ID)

	// mark deployment 4 as skipped
	taskContext.InventoryManager().AddSkippedApply(testDeployment4ID)

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
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.Inventory{
		Status: inventory.InventoryStatus{
			Objects: []inventory.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSucceeded,
					Reconcile:       inventory.ReconcileSucceeded,
					UID:             "unused",
					Generation:      1,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSucceeded,
					Reconcile:       inventory.ReconcileTimeout,
					UID:             "unused",
					Generation:      1,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment3ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationFailed,
					Reconcile:       inventory.ReconcileSkipped,
				},
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment4ID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSkipped,
					Reconcile:       inventory.ReconcileSkipped,
				},
			},
		},
	}
	testutil.AssertEqual(t, &expectedInventory, taskContext.InventoryManager().Inventory())
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
	taskContext.InventoryManager().AddSuccessfulApply(testDeploymentID, "unused", 1)

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
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.Inventory{
		Status: inventory.InventoryStatus{
			Objects: []inventory.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeploymentID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSucceeded,
					Reconcile:       inventory.ReconcileSucceeded,
					UID:             "unused",
					Generation:      1,
				},
			},
		},
	}
	testutil.AssertEqual(t, &expectedInventory, taskContext.InventoryManager().Inventory())
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

	// mark the deployment as applied
	taskContext.InventoryManager().AddSuccessfulApply(testDeploymentID, "unused", 1)

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
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.Inventory{
		Status: inventory.InventoryStatus{
			Objects: []inventory.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeploymentID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSucceeded,
					Reconcile:       inventory.ReconcilePending,
					UID:             "unused",
					Generation:      1,
				},
			},
		},
	}
	testutil.AssertEqual(t, &expectedInventory, taskContext.InventoryManager().Inventory())
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
	taskContext.InventoryManager().AddSuccessfulApply(testDeploymentID, "unused", 1)

	// run task async, to let the test collect events
	go func() {
		// start the task
		task.Start(taskContext)

		// wait a bit
		time.Sleep(1 * time.Second)

		// mark the deployment as Current
		resourceCache.Put(testDeploymentID, cache.ResourceStatus{
			Resource: withGeneration(testDeployment, 1),
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
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedResults := []TaskResult{
		{}, // Empty result means success
	}
	assert.Equal(t, expectedResults, receivedResults)

	expectedInventory := inventory.Inventory{
		Status: inventory.InventoryStatus{
			Objects: []inventory.ObjectStatus{
				{
					ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeploymentID),
					Strategy:        inventory.ActuationStrategyApply,
					Actuation:       inventory.ActuationSucceeded,
					Reconcile:       inventory.ReconcileSucceeded,
					UID:             "unused",
					Generation:      1,
				},
			},
		},
	}
	testutil.AssertEqual(t, &expectedInventory, taskContext.InventoryManager().Inventory())
}

func TestWaitTask_Failed(t *testing.T) {
	taskName := "wait-1"
	testDeployment1ID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment1 := testutil.Unstructured(t, testDeployment1YAML)
	testDeployment2ID := testutil.ToIdentifier(t, testDeployment2YAML)
	testDeployment2 := testutil.Unstructured(t, testDeployment2YAML)

	testCases := map[string]struct {
		configureTaskContextFunc func(taskContext *TaskContext)
		eventsFunc               func(*cache.ResourceCacheMap, *WaitTask, *TaskContext)
		waitTimeout              time.Duration
		expectedEvents           []event.Event
		expectedInventory        *inventory.Inventory
	}{
		"continue on failed if others InProgress": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID, "unused", 1)
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID, "unused", 1)
			},
			eventsFunc: func(resourceCache *cache.ResourceCacheMap, task *WaitTask, taskContext *TaskContext) {
				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: testDeployment1,
					Status:   status.FailedStatus,
				})
				task.StatusUpdate(taskContext, testDeployment1ID)

				resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
					Resource: testDeployment2,
					Status:   status.InProgressStatus,
				})
				task.StatusUpdate(taskContext, testDeployment2ID)

				resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
					Resource: testDeployment2,
					Status:   status.CurrentStatus,
				})
				task.StatusUpdate(taskContext, testDeployment2ID)
			},
			waitTimeout: 2 * time.Second,
			expectedEvents: []event.Event{
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
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Operation:  event.ReconcileFailed,
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
			},
			expectedInventory: &inventory.Inventory{
				Status: inventory.InventoryStatus{
					Objects: []inventory.ObjectStatus{
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileFailed,
							UID:             "unused",
							Generation:      1,
						},
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileSucceeded,
							UID:             "unused",
							Generation:      1,
						},
					},
				},
			},
		},
		"complete wait task is last resource becomes failed": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID, "unused", 1)
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID, "unused", 1)
			},
			eventsFunc: func(resourceCache *cache.ResourceCacheMap, task *WaitTask, taskContext *TaskContext) {
				resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
					Resource: testDeployment2,
					Status:   status.CurrentStatus,
				})
				task.StatusUpdate(taskContext, testDeployment2ID)

				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: testDeployment1,
					Status:   status.FailedStatus,
				})
				task.StatusUpdate(taskContext, testDeployment1ID)
			},
			waitTimeout: 2 * time.Second,
			expectedEvents: []event.Event{
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
				// deployment2 current
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Operation:  event.Reconciled,
					},
				},
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Operation:  event.ReconcileFailed,
					},
				},
			},
			expectedInventory: &inventory.Inventory{
				Status: inventory.InventoryStatus{
					Objects: []inventory.ObjectStatus{
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileFailed,
							UID:             "unused",
							Generation:      1,
						},
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileSucceeded,
							UID:             "unused",
							Generation:      1,
						},
					},
				},
			},
		},
		"failed resource can become current": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID, "unused", 1)
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID, "unused", 1)
			},
			eventsFunc: func(resourceCache *cache.ResourceCacheMap, task *WaitTask, taskContext *TaskContext) {
				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: testDeployment1,
					Status:   status.FailedStatus,
				})
				task.StatusUpdate(taskContext, testDeployment1ID)

				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: testDeployment1,
					Status:   status.CurrentStatus,
				})
				task.StatusUpdate(taskContext, testDeployment1ID)

				resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
					Resource: testDeployment2,
					Status:   status.CurrentStatus,
				})
				task.StatusUpdate(taskContext, testDeployment2ID)
			},
			waitTimeout: 2 * time.Second,
			expectedEvents: []event.Event{
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
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Operation:  event.ReconcileFailed,
					},
				},
				// deployment1 is current
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
			},
			expectedInventory: &inventory.Inventory{
				Status: inventory.InventoryStatus{
					Objects: []inventory.ObjectStatus{
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileSucceeded,
							UID:             "unused",
							Generation:      1,
						},
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileSucceeded,
							UID:             "unused",
							Generation:      1,
						},
					},
				},
			},
		},
		"failed resource can become InProgress": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID, "unused", 1)
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID, "unused", 1)
			},
			eventsFunc: func(resourceCache *cache.ResourceCacheMap, task *WaitTask, taskContext *TaskContext) {
				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: testDeployment1,
					Status:   status.FailedStatus,
				})
				task.StatusUpdate(taskContext, testDeployment1ID)

				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: testDeployment1,
					Status:   status.InProgressStatus,
				})
				task.StatusUpdate(taskContext, testDeployment1ID)

				resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
					Resource: testDeployment2,
					Status:   status.CurrentStatus,
				})
				task.StatusUpdate(taskContext, testDeployment2ID)
			},
			waitTimeout: 2 * time.Second,
			expectedEvents: []event.Event{
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
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Operation:  event.ReconcileFailed,
					},
				},
				// deployment1 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Operation:  event.ReconcilePending,
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
				// deployment1 timed out
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Operation:  event.ReconcileTimeout,
					},
				},
			},
			expectedInventory: &inventory.Inventory{
				Status: inventory.InventoryStatus{
					Objects: []inventory.ObjectStatus{
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileTimeout,
							UID:             "unused",
							Generation:      1,
						},
						{
							ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
							Strategy:        inventory.ActuationStrategyApply,
							Actuation:       inventory.ActuationSucceeded,
							Reconcile:       inventory.ReconcileSucceeded,
							UID:             "unused",
							Generation:      1,
						},
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ids := object.ObjMetadataSet{
				testDeployment1ID,
				testDeployment2ID,
			}
			task := NewWaitTask(taskName, ids, AllCurrent,
				tc.waitTimeout, testutil.NewFakeRESTMapper())

			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := NewTaskContext(eventChannel, resourceCache)
			defer close(eventChannel)

			tc.configureTaskContextFunc(taskContext)

			// run task async, to let the test collect events
			go func() {
				// start the task
				task.Start(taskContext)

				tc.eventsFunc(resourceCache, task, taskContext)
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

			testutil.AssertEqual(t, tc.expectedEvents, receivedEvents,
				"Actual events (%d) do not match expected events (%d)",
				len(receivedEvents), len(tc.expectedEvents))

			testutil.AssertEqual(t, tc.expectedInventory, taskContext.InventoryManager().Inventory())
		})
	}
}

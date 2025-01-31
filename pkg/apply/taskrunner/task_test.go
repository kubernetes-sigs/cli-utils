// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
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
	taskContext := NewTaskContext(t.Context(), eventChannel, resourceCache)
	defer close(eventChannel)

	// Update metadata on successfully applied objects
	testDeployment1.SetUID("a")
	testDeployment1.SetGeneration(1)
	testDeployment2.SetUID("b")
	testDeployment2.SetGeneration(1)

	// mark deployment 1 & 2 as apply succeeded
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID,
		testDeployment1.GetUID(), testDeployment1.GetGeneration())
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID,
		testDeployment2.GetUID(), testDeployment2.GetGeneration())

	// mark deployment 3 as apply failed
	taskContext.InventoryManager().AddFailedApply(testDeployment3ID)

	// mark deployment 4 as apply skipped
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
				Status:     event.ReconcilePending,
			},
		},
		// deployment2 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Status:     event.ReconcilePending,
			},
		},
		// deployment3 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment3ID,
				Status:     event.ReconcileSkipped,
			},
		},
		// deployment4 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment4ID,
				Status:     event.ReconcileSkipped,
			},
		},
		// current events next, in the order of status updates
		// deployment1 current
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment1ID,
				Status:     event.ReconcileSuccessful,
			},
		},
		// deployment2 current
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Status:     event.ReconcileSuccessful,
			},
		},
	}
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.BaseInventory{
		ObjStatuses: []actuation.ObjectStatus{
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSucceeded,
				Reconcile:       actuation.ReconcileSucceeded,
				UID:             testDeployment1.GetUID(),
				Generation:      testDeployment1.GetGeneration(),
			},
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSucceeded,
				Reconcile:       actuation.ReconcileSucceeded,
				UID:             testDeployment2.GetUID(),
				Generation:      testDeployment2.GetGeneration(),
			},
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment3ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationFailed,
				Reconcile:       actuation.ReconcileSkipped,
			},
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment4ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSkipped,
				Reconcile:       actuation.ReconcileSkipped,
			},
		},
	}
	testutil.AssertEqual(t, expectedInventory, taskContext.InventoryManager().Inventory())
}

func TestWaitTask_Timeout(t *testing.T) {
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
	taskName := "wait-2"
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(t.Context(), eventChannel, resourceCache)
	defer close(eventChannel)

	// Update metadata on successfully applied objects
	testDeployment1.SetUID("a")
	testDeployment1.SetGeneration(1)
	testDeployment2.SetUID("b")
	testDeployment2.SetGeneration(1)

	// mark deployment 1 & 2 as apply succeeded
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID,
		testDeployment1.GetUID(), testDeployment1.GetGeneration())
	taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID,
		testDeployment2.GetUID(), testDeployment2.GetGeneration())

	// mark deployment 3 as apply failed
	taskContext.InventoryManager().AddFailedApply(testDeployment3ID)

	// mark deployment 4 as apply skipped
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
				Status:     event.ReconcilePending,
			},
		},
		// deployment2 pending
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Status:     event.ReconcilePending,
			},
		},
		// deployment3 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment3ID,
				Status:     event.ReconcileSkipped,
			},
		},
		// deployment4 skipped
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment4ID,
				Status:     event.ReconcileSkipped,
			},
		},
		// current events next, in the order of status updates
		// deployment1 current
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment1ID,
				Status:     event.ReconcileSuccessful,
			},
		},
		// timeout events last, in the order provided to the WaitTask
		// deployment2 timeout
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeployment2ID,
				Status:     event.ReconcileTimeout,
			},
		},
	}
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.BaseInventory{
		ObjStatuses: []actuation.ObjectStatus{
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSucceeded,
				Reconcile:       actuation.ReconcileSucceeded,
				UID:             testDeployment1.GetUID(),
				Generation:      testDeployment1.GetGeneration(),
			},
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSucceeded,
				Reconcile:       actuation.ReconcileTimeout,
				UID:             testDeployment2.GetUID(),
				Generation:      testDeployment2.GetGeneration(),
			},
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment3ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationFailed,
				Reconcile:       actuation.ReconcileSkipped,
			},
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment4ID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSkipped,
				Reconcile:       actuation.ReconcileSkipped,
			},
		},
	}
	testutil.AssertEqual(t, expectedInventory, taskContext.InventoryManager().Inventory())
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
	taskContext := NewTaskContext(t.Context(), eventChannel, resourceCache)
	defer close(eventChannel)

	// Update metadata on successfully applied objects
	testDeployment.SetUID("a")
	testDeployment.SetGeneration(1)

	// mark deployment as apply succeeded
	taskContext.InventoryManager().AddSuccessfulApply(testDeploymentID,
		testDeployment.GetUID(), testDeployment.GetGeneration())

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
				Status:     event.ReconcileSuccessful,
			},
		},
	}
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.BaseInventory{
		ObjStatuses: []actuation.ObjectStatus{
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeploymentID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSucceeded,
				Reconcile:       actuation.ReconcileSucceeded,
				UID:             testDeployment.GetUID(),
				Generation:      testDeployment.GetGeneration(),
			},
		},
	}
	testutil.AssertEqual(t, expectedInventory, taskContext.InventoryManager().Inventory())
}

func TestWaitTask_Cancel(t *testing.T) {
	testDeploymentID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment := testutil.Unstructured(t, testDeployment1YAML)
	ids := object.ObjMetadataSet{
		testDeploymentID,
	}
	waitTimeout := 5 * time.Second
	taskName := "wait-4"
	task := NewWaitTask(taskName, ids, AllCurrent,
		waitTimeout, testutil.NewFakeRESTMapper())

	eventChannel := make(chan event.Event)
	resourceCache := cache.NewResourceCacheMap()
	taskContext := NewTaskContext(t.Context(), eventChannel, resourceCache)
	defer close(eventChannel)

	// Update metadata on successfully applied objects
	testDeployment.SetUID("a")
	testDeployment.SetGeneration(1)

	// mark deployment as apply succeeded
	taskContext.InventoryManager().AddSuccessfulApply(testDeploymentID,
		testDeployment.GetUID(), testDeployment.GetGeneration())

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
				Status:     event.ReconcilePending,
			},
		},
	}
	testutil.AssertEqual(t, expectedEvents, receivedEvents,
		"Actual events (%d) do not match expected events (%d)",
		len(receivedEvents), len(expectedEvents))

	expectedInventory := inventory.BaseInventory{
		ObjStatuses: []actuation.ObjectStatus{
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeploymentID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSucceeded,
				Reconcile:       actuation.ReconcilePending,
				UID:             testDeployment.GetUID(),
				Generation:      testDeployment.GetGeneration(),
			},
		},
	}
	testutil.AssertEqual(t, expectedInventory, taskContext.InventoryManager().Inventory())
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
	taskContext := NewTaskContext(t.Context(), eventChannel, resourceCache)
	defer close(eventChannel)

	// Update metadata on successfully applied objects
	testDeployment.SetUID("a")
	testDeployment.SetGeneration(1)

	// mark deployment as apply succeeded
	taskContext.InventoryManager().AddSuccessfulApply(testDeploymentID,
		testDeployment.GetUID(), testDeployment.GetGeneration())

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
				Status:     event.ReconcilePending,
			},
		},
		// deployment1 reconciled
		{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  taskName,
				Identifier: testDeploymentID,
				Status:     event.ReconcileSuccessful,
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

	expectedInventory := inventory.BaseInventory{
		ObjStatuses: []actuation.ObjectStatus{
			{
				ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeploymentID),
				Strategy:        actuation.ActuationStrategyApply,
				Actuation:       actuation.ActuationSucceeded,
				Reconcile:       actuation.ReconcileSucceeded,
				UID:             testDeployment.GetUID(),
				Generation:      testDeployment.GetGeneration(),
			},
		},
	}
	testutil.AssertEqual(t, expectedInventory, taskContext.InventoryManager().Inventory())
}

func TestWaitTask_Failed(t *testing.T) {
	taskName := "wait-6"
	testDeployment1ID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment1 := testutil.Unstructured(t, testDeployment1YAML)
	testDeployment2ID := testutil.ToIdentifier(t, testDeployment2YAML)
	testDeployment2 := testutil.Unstructured(t, testDeployment2YAML)

	// Update metadata on successfully applied objects
	testDeployment1.SetUID("a")
	testDeployment1.SetGeneration(1)
	testDeployment2.SetUID("b")
	testDeployment2.SetGeneration(1)

	testCases := map[string]struct {
		configureTaskContextFunc func(taskContext *TaskContext)
		eventsFunc               func(*cache.ResourceCacheMap, *WaitTask, *TaskContext)
		waitTimeout              time.Duration
		expectedEvents           []event.Event
		expectedInventory        inventory.BaseInventory
	}{
		"continue on failed if others InProgress": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				// mark deployment as apply succeeded
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID,
					testDeployment1.GetUID(), testDeployment1.GetGeneration())
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID,
					testDeployment2.GetUID(), testDeployment2.GetGeneration())
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
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcilePending,
					},
				},
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileFailed,
					},
				},
				// deployment2 current
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcileSuccessful,
					},
				},
			},
			expectedInventory: inventory.BaseInventory{
				ObjStatuses: []actuation.ObjectStatus{
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileFailed,
						UID:             testDeployment1.GetUID(),
						Generation:      testDeployment1.GetGeneration(),
					},
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileSucceeded,
						UID:             testDeployment2.GetUID(),
						Generation:      testDeployment2.GetGeneration(),
					},
				},
			},
		},
		"complete wait task is last resource becomes failed": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				// mark deployment as apply succeeded
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID,
					testDeployment1.GetUID(), testDeployment1.GetGeneration())
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID,
					testDeployment2.GetUID(), testDeployment2.GetGeneration())
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
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 current
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcileSuccessful,
					},
				},
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileFailed,
					},
				},
			},
			expectedInventory: inventory.BaseInventory{
				ObjStatuses: []actuation.ObjectStatus{
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileFailed,
						UID:             testDeployment1.GetUID(),
						Generation:      testDeployment1.GetGeneration(),
					},
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileSucceeded,
						UID:             testDeployment2.GetUID(),
						Generation:      testDeployment2.GetGeneration(),
					},
				},
			},
		},
		"failed resource can become current": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				// mark deployment as apply succeeded
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID,
					testDeployment1.GetUID(), testDeployment1.GetGeneration())
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID,
					testDeployment2.GetUID(), testDeployment2.GetGeneration())
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
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcilePending,
					},
				},
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileFailed,
					},
				},
				// deployment1 is current
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileSuccessful,
					},
				},
				// deployment2 current
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcileSuccessful,
					},
				},
			},
			expectedInventory: inventory.BaseInventory{
				ObjStatuses: []actuation.ObjectStatus{
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileSucceeded,
						UID:             testDeployment1.GetUID(),
						Generation:      testDeployment1.GetGeneration(),
					},
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileSucceeded,
						UID:             testDeployment2.GetUID(),
						Generation:      testDeployment2.GetGeneration(),
					},
				},
			},
		},
		"failed resource can become InProgress": {
			configureTaskContextFunc: func(taskContext *TaskContext) {
				// mark deployment as apply succeeded
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID,
					testDeployment1.GetUID(), testDeployment1.GetGeneration())
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID,
					testDeployment2.GetUID(), testDeployment2.GetGeneration())
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
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcilePending,
					},
				},
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileFailed,
					},
				},
				// deployment1 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 current
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcileSuccessful,
					},
				},
				// deployment1 timed out
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileTimeout,
					},
				},
			},
			expectedInventory: inventory.BaseInventory{
				ObjStatuses: []actuation.ObjectStatus{
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileTimeout,
						UID:             testDeployment1.GetUID(),
						Generation:      testDeployment1.GetGeneration(),
					},
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileSucceeded,
						UID:             testDeployment2.GetUID(),
						Generation:      testDeployment2.GetGeneration(),
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
			taskContext := NewTaskContext(t.Context(), eventChannel, resourceCache)
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

func TestWaitTask_UIDChanged(t *testing.T) {
	taskName := "wait-7"
	testDeployment1ID := testutil.ToIdentifier(t, testDeployment1YAML)
	testDeployment1 := testutil.Unstructured(t, testDeployment1YAML)
	testDeployment2ID := testutil.ToIdentifier(t, testDeployment2YAML)
	testDeployment2 := testutil.Unstructured(t, testDeployment2YAML)

	// Update metadata on successfully applied objects
	testDeployment1.SetUID("a")
	testDeployment1.SetGeneration(1)
	testDeployment2.SetUID("b")
	testDeployment2.SetGeneration(1)

	replacedDeployment1 := testDeployment1.DeepCopy()
	replacedDeployment1.SetUID("replaced")

	testCases := map[string]struct {
		condition                Condition
		configureTaskContextFunc func(taskContext *TaskContext)
		eventsFunc               func(*cache.ResourceCacheMap, *WaitTask, *TaskContext)
		waitTimeout              time.Duration
		expectedEvents           []event.Event
		expectedInventory        inventory.BaseInventory
	}{
		"UID changed after apply means reconcile failure": {
			condition: AllCurrent,
			configureTaskContextFunc: func(taskContext *TaskContext) {
				// mark deployment as apply succeeded
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment1ID,
					testDeployment1.GetUID(), testDeployment1.GetGeneration())
				taskContext.InventoryManager().AddSuccessfulApply(testDeployment2ID,
					testDeployment2.GetUID(), testDeployment2.GetGeneration())
			},
			eventsFunc: func(resourceCache *cache.ResourceCacheMap, task *WaitTask, taskContext *TaskContext) {
				// any status update after apply success should trigger failure if the UID changed
				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: replacedDeployment1,
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
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcilePending,
					},
				},
				// deployment1 is failed
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileFailed,
					},
				},
				// deployment2 current
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcileSuccessful,
					},
				},
			},
			expectedInventory: inventory.BaseInventory{
				ObjStatuses: []actuation.ObjectStatus{
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						// UID change causes failure after apply
						Reconcile: actuation.ReconcileFailed,
						// Recorded UID should be from the applied object, not the new replacement
						UID:        testDeployment1.GetUID(),
						Generation: testDeployment1.GetGeneration(),
					},
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
						Strategy:        actuation.ActuationStrategyApply,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileSucceeded,
						UID:             testDeployment2.GetUID(),
						Generation:      testDeployment2.GetGeneration(),
					},
				},
			},
		},
		"UID changed after delete means reconcile success": {
			condition: AllNotFound,
			configureTaskContextFunc: func(taskContext *TaskContext) {
				// mark deployment as apply succeeded
				taskContext.InventoryManager().AddSuccessfulDelete(testDeployment1ID,
					testDeployment1.GetUID())
				taskContext.InventoryManager().AddSuccessfulDelete(testDeployment2ID,
					testDeployment2.GetUID())
			},
			eventsFunc: func(resourceCache *cache.ResourceCacheMap, task *WaitTask, taskContext *TaskContext) {
				// any status update after delete should trigger success if the UID changed
				resourceCache.Put(testDeployment1ID, cache.ResourceStatus{
					Resource: replacedDeployment1,
					Status:   status.InProgressStatus,
				})
				task.StatusUpdate(taskContext, testDeployment1ID)

				resourceCache.Put(testDeployment2ID, cache.ResourceStatus{
					Resource: testDeployment2,
					Status:   status.NotFoundStatus,
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
						Status:     event.ReconcilePending,
					},
				},
				// deployment2 pending
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcilePending,
					},
				},
				// deployment1 is replaced
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment1ID,
						Status:     event.ReconcileSuccessful,
					},
				},
				// deployment2 not found
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  taskName,
						Identifier: testDeployment2ID,
						Status:     event.ReconcileSuccessful,
					},
				},
			},
			expectedInventory: inventory.BaseInventory{
				ObjStatuses: []actuation.ObjectStatus{
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment1ID),
						Strategy:        actuation.ActuationStrategyDelete,
						Actuation:       actuation.ActuationSucceeded,
						// UID change causes success after delete
						Reconcile: actuation.ReconcileSucceeded,
						// Recorded UID should be from the deleted object, not the new replacement
						UID: testDeployment1.GetUID(),
						// Deleted generation is unknown
					},
					{
						ObjectReference: inventory.ObjectReferenceFromObjMetadata(testDeployment2ID),
						Strategy:        actuation.ActuationStrategyDelete,
						Actuation:       actuation.ActuationSucceeded,
						Reconcile:       actuation.ReconcileSucceeded,
						UID:             testDeployment2.GetUID(),
						// Deleted generation is unknown
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
			task := NewWaitTask(taskName, ids, tc.condition,
				tc.waitTimeout, testutil.NewFakeRESTMapper())

			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			taskContext := NewTaskContext(t.Context(), eventChannel, resourceCache)
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

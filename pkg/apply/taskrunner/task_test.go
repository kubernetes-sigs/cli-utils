// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	kstatus "sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestWaitTask_TaskTimeout(t *testing.T) {
	// ensure conditions are not met, or task with exit early
	task := NewWaitTask(
		"wait",
		[]object.ObjMetadata{depID},
		AllCurrent,
		2*time.Second,
		testutil.NewFakeRESTMapper(),
	)

	eventChannel := make(chan event.Event)
	taskContext := NewTaskContext(context.TODO(), eventChannel)
	defer close(eventChannel)

	task.Start(taskContext)

	timer := time.NewTimer(3 * time.Second)

	select {
	case res := <-taskContext.TaskChannel():
		if _, ok := IsTimeoutError(res.Err); !ok {
			t.Errorf("expected timeout error, but got %v", res.Err)
		}
		expected := &TimeoutError{
			Identifiers: []object.ObjMetadata{depID},
			Timeout:     2 * time.Second,
			Condition:   AllCurrent,
		}
		require.Equal(t, res.Err.Error(), expected.Error())
	case <-timer.C:
		t.Errorf("expected timeout to trigger, but it didn't")
	}
}

func TestWaitTask_ContextCancelled(t *testing.T) {
	// ensure conditions are not met, or task with exit early
	task := NewWaitTask(
		"wait",
		[]object.ObjMetadata{depID},
		AllCurrent,
		2*time.Second,
		testutil.NewFakeRESTMapper(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	eventChannel := make(chan event.Event)
	taskContext := NewTaskContext(ctx, eventChannel)
	defer close(eventChannel)
	defer cancel()

	task.Start(taskContext)

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	timer := time.NewTimer(3 * time.Second)

	select {
	case res := <-taskContext.TaskChannel():
		require.ErrorIs(t, res.Err, context.Canceled)
	case <-timer.C:
		t.Errorf("unexpected timeout")
	}
}

func TestWaitTask_ContextTimeout(t *testing.T) {
	// ensure conditions are not met, or task with exit early
	task := NewWaitTask(
		"wait",
		[]object.ObjMetadata{depID},
		AllCurrent,
		2*time.Second,
		testutil.NewFakeRESTMapper(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	eventChannel := make(chan event.Event)
	taskContext := NewTaskContext(ctx, eventChannel)
	defer close(eventChannel)
	defer cancel()

	task.Start(taskContext)

	timer := time.NewTimer(3 * time.Second)

	select {
	case res := <-taskContext.TaskChannel():
		require.ErrorIs(t, res.Err, context.DeadlineExceeded)
	case <-timer.C:
		t.Errorf("unexpected timeout")
	}
}

// TestWaitTask_OnStatusEvent tests that OnStatusEvent with the right status in
// the ResourceStatusCollector triggers a TaskResult on the TaskChannel.
func TestWaitTask_OnStatusEvent(t *testing.T) {
	// ensure conditions are not met, or task with exit early.
	task := NewWaitTask(
		"wait",
		[]object.ObjMetadata{depID},
		AllCurrent,
		0*time.Second,
		testutil.NewFakeRESTMapper(),
	)

	eventChannel := make(chan event.Event)
	taskContext := NewTaskContext(context.TODO(), eventChannel)
	taskContext.taskChannel = make(chan TaskResult, 10)
	defer close(eventChannel)

	task.Start(taskContext)

	go func() {
		klog.V(5).Infof("status event %d", 1)
		taskContext.ResourceStatusCollector().Put(depID, ResourceStatus{
			CurrentStatus: kstatus.CurrentStatus,
			Generation:    1,
		})
		task.OnStatusEvent(taskContext, event.StatusEvent{})
		klog.V(5).Infof("status event %d handled", 1)
	}()

	timer := time.NewTimer(4 * time.Second)

	select {
	case res := <-taskContext.TaskChannel():
		require.NoError(t, res.Err)
	case <-timer.C:
		t.Errorf("unexpected timeout")
	}
}

// TestWaitTask_SingleTaskResult tests that WaitTask can handle more than one
// call to OnStatusEvent and still only send one result on the TaskChannel.
func TestWaitTask_SingleTaskResult(t *testing.T) {
	// ensure conditions are not met, or task with exit early.
	task := NewWaitTask(
		"wait",
		[]object.ObjMetadata{depID},
		AllCurrent,
		0*time.Second,
		testutil.NewFakeRESTMapper(),
	)

	eventChannel := make(chan event.Event)
	taskContext := NewTaskContext(context.TODO(), eventChannel)
	taskContext.taskChannel = make(chan TaskResult, 10)
	defer close(eventChannel)

	task.Start(taskContext)

	var completeWg sync.WaitGroup
	completeWg.Add(1)
	go func() {
		defer completeWg.Done()
		klog.V(5).Info("waiting for task result")
		res := <-taskContext.TaskChannel()
		klog.V(5).Infof("received task result: %v", res.Err)
		require.NoError(t, res.Err)
	}()
	completeWg.Add(4)
	go func() {
		for i := 0; i < 4; i++ {
			index := i
			go func() {
				defer completeWg.Done()
				time.Sleep(time.Duration(index) * time.Second)
				klog.V(5).Infof("status event %d", index)
				if index > 2 {
					taskContext.ResourceStatusCollector().Put(depID, ResourceStatus{
						CurrentStatus: kstatus.CurrentStatus,
						Generation:    1,
					})
				}
				task.OnStatusEvent(taskContext, event.StatusEvent{})
				klog.V(5).Infof("status event %d handled", index)
			}()
		}
	}()
	completeWg.Wait()

	timer := time.NewTimer(4 * time.Second)

	select {
	case <-taskContext.TaskChannel():
		t.Errorf("expected only one result on taskChannel, but got more")
	case <-timer.C:
		return
	}
}

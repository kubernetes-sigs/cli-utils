// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"sync"
	"testing"
	"time"

	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestWaitTask_TimeoutTriggered(t *testing.T) {
	task := NewWaitTask([]object.ObjMetadata{}, AllCurrent, 2*time.Second)

	taskChannel := make(chan TaskResult)
	defer close(taskChannel)

	task.Start(taskChannel)

	timer := time.NewTimer(3 * time.Second)

	select {
	case res := <-taskChannel:
		if res.Err == nil || !IsTimeoutError(res.Err) {
			t.Errorf("expected timeout error, but got %v", res.Err)
		}
		return
	case <-timer.C:
		t.Errorf("expected timeout to trigger, but it didn't")
	}
}

func TestWaitTask_TimeoutCancelled(t *testing.T) {
	task := NewWaitTask([]object.ObjMetadata{}, AllCurrent, 2*time.Second)

	taskChannel := make(chan TaskResult)
	defer close(taskChannel)

	task.Start(taskChannel)
	task.ClearTimeout()
	timer := time.NewTimer(3 * time.Second)

	select {
	case res := <-taskChannel:
		t.Errorf("didn't expect timeout error, but got %v", res.Err)
	case <-timer.C:
		return
	}
}

func TestWaitTask_SingleTaskResult(t *testing.T) {
	task := NewWaitTask([]object.ObjMetadata{}, AllCurrent, 2*time.Second)

	taskChannel := make(chan TaskResult, 10)

	var completeWg sync.WaitGroup

	for i := 0; i < 10; i++ {
		completeWg.Add(1)
		go func() {
			defer completeWg.Done()
			task.complete(taskChannel)
		}()
	}
	completeWg.Wait()
	close(taskChannel)

	resultCount := 0
	for range taskChannel {
		resultCount++
	}
	if resultCount != 1 {
		t.Errorf("expected 1 result, but got %d", resultCount)
	}
}

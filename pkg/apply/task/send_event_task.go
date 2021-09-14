// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
)

// SendEventTask is an implementation of the Task interface
// that will send the provided event on the eventChannel when
// executed.
type SendEventTask struct {
	Event event.Event
}

// Start start a separate goroutine that will send the
// event and then push a TaskResult on the taskChannel to
// signal to the taskrunner that the task is completed.
func (s *SendEventTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		taskContext.EventChannel() <- s.Event
		taskContext.TaskChannel() <- taskrunner.TaskResult{}
	}()
}

// OnStatusEvent is not supported by SendEventTask.
func (s *SendEventTask) OnStatusEvent(taskContext *taskrunner.TaskContext, e event.StatusEvent) {}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"sigs.k8s.io/cli-utils/pkg/apply/event"
)

// NewTaskContext returns a new TaskContext
func NewTaskContext(eventChannel chan event.Event) *TaskContext {
	return &TaskContext{
		taskChannel:  make(chan TaskResult),
		eventChannel: eventChannel,
	}
}

// TaskContext defines a context that is passed between all
// the tasks that is in a taskqueue.
type TaskContext struct {
	taskChannel chan TaskResult

	eventChannel chan event.Event
}

func (tc *TaskContext) TaskChannel() chan TaskResult {
	return tc.taskChannel
}

func (tc *TaskContext) EventChannel() chan event.Event {
	return tc.eventChannel
}

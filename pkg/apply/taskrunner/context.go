// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// NewTaskContext returns a new TaskContext
func NewTaskContext(eventChannel chan event.Event) *TaskContext {
	return &TaskContext{
		taskChannel:      make(chan TaskResult),
		eventChannel:     eventChannel,
		appliedResources: make(map[object.ObjMetadata]applyInfo),
	}
}

// TaskContext defines a context that is passed between all
// the tasks that is in a taskqueue.
type TaskContext struct {
	taskChannel chan TaskResult

	eventChannel chan event.Event

	appliedResources map[object.ObjMetadata]applyInfo
}

func (tc *TaskContext) TaskChannel() chan TaskResult {
	return tc.taskChannel
}

func (tc *TaskContext) EventChannel() chan event.Event {
	return tc.eventChannel
}

// ResourceApplied updates the context with information about the
// resource identified by the provided id. Currently, we keep information
// about the generation of the resource after the apply operation completed.
func (tc *TaskContext) ResourceApplied(id object.ObjMetadata, gen int64) {
	tc.appliedResources[id] = applyInfo{
		generation: gen,
	}
}

// ResourceGeneration looks up the generation of the given resource
// after it was applied.
func (tc *TaskContext) ResourceGeneration(id object.ObjMetadata) int64 {
	ai, found := tc.appliedResources[id]
	if !found {
		return 0
	}
	return ai.generation
}

// applyInfo captures information about resources that have been
// applied. This is captured in the TaskContext so other tasks
// running later might use this information.
type applyInfo struct {
	// generation captures the "version" of the resource after it
	// has been applied. Generation is a monotonically increasing number
	// that the APIServer increases every time the desired state of a
	// resource changes.
	generation int64
}

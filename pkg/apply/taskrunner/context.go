// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// NewTaskContext returns a new TaskContext
func NewTaskContext(eventChannel chan event.Event) *TaskContext {
	return &TaskContext{
		taskChannel:      make(chan TaskResult),
		eventChannel:     eventChannel,
		appliedResources: make(map[object.ObjMetadata]applyInfo),
		failedResources:  make(map[object.ObjMetadata]struct{}),
	}
}

// TaskContext defines a context that is passed between all
// the tasks that is in a taskqueue.
type TaskContext struct {
	taskChannel chan TaskResult

	eventChannel chan event.Event

	appliedResources map[object.ObjMetadata]applyInfo

	// failedResources records the IDs of resources that are failed during applying and pruning.
	failedResources map[object.ObjMetadata]struct{}
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
func (tc *TaskContext) ResourceApplied(id object.ObjMetadata, uid types.UID, gen int64) {
	tc.appliedResources[id] = applyInfo{
		generation: gen,
		uid:        uid,
	}
}

// ResourceUID looks up the UID of the given resource
func (tc *TaskContext) ResourceUID(id object.ObjMetadata) (types.UID, bool) {
	ai, found := tc.appliedResources[id]
	if !found {
		return "", false
	}
	return ai.uid, true
}

// AppliedResources returns all the objects (as ObjMetadata) that
// were added as applied resources to the TaskContext.
func (tc *TaskContext) AppliedResources() []object.ObjMetadata {
	all := make([]object.ObjMetadata, 0, len(tc.appliedResources))
	for r := range tc.appliedResources {
		all = append(all, r)
	}
	return all
}

// AllResourceUIDs returns a set with the UIDs of all the resources in the
// context.
func (tc *TaskContext) AllResourceUIDs() sets.String {
	uids := sets.NewString()
	for _, ai := range tc.appliedResources {
		uid := strings.TrimSpace(string(ai.uid))
		if uid != "" {
			uids.Insert(uid)
		}
	}
	return uids
}

// ResourceGeneration looks up the generation of the given resource
// after it was applied.
func (tc *TaskContext) ResourceGeneration(id object.ObjMetadata) (int64, bool) {
	ai, found := tc.appliedResources[id]
	if !found {
		return 0, false
	}
	return ai.generation, true
}

func (tc *TaskContext) ResourceFailed(id object.ObjMetadata) bool {
	_, found := tc.failedResources[id]
	return found
}

func (tc *TaskContext) CaptureResourceFailure(id object.ObjMetadata) {
	tc.failedResources[id] = struct{}{}
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

	// uid captures the uid of the resource that has been applied.
	uid types.UID
}

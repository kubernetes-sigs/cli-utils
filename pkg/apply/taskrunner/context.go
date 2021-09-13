// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
		appliedResources: make(map[object.ObjMetadata]*unstructured.Unstructured),
		failedResources:  make(map[object.ObjMetadata]struct{}),
		pruneFailures:    make(map[object.ObjMetadata]struct{}),
	}
}

// TaskContext defines a context that is passed between all
// the tasks that is in a taskqueue.
type TaskContext struct {
	taskChannel chan TaskResult

	eventChannel chan event.Event

	// map of successfully applied objects keyed by object id
	appliedResources map[object.ObjMetadata]*unstructured.Unstructured

	// failedResources records the IDs of resources that are failed during applying.
	failedResources map[object.ObjMetadata]struct{}

	// pruneFailures records the IDs of resources that are failed during pruning.
	pruneFailures map[object.ObjMetadata]struct{}
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
func (tc *TaskContext) ResourceApplied(id object.ObjMetadata, obj *unstructured.Unstructured) {
	tc.appliedResources[id] = obj
}

// GetAppliedResource returns a pointer to a successfully applied object
// stored in the TaskContext identified by the passed id, nil otherwise.
func (tc *TaskContext) GetAppliedResource(id object.ObjMetadata) *unstructured.Unstructured {
	return tc.appliedResources[id]
}

// ResourceUID looks up the UID of the given resource
func (tc *TaskContext) ResourceUID(id object.ObjMetadata) (types.UID, bool) {
	obj, found := tc.appliedResources[id]
	if !found {
		return "", false
	}
	return obj.GetUID(), true
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

// AppliedResourceUIDs returns a set with the UIDs of all the
// successfully applied resources.
func (tc *TaskContext) AppliedResourceUIDs() sets.String {
	uids := sets.NewString()
	for _, obj := range tc.appliedResources {
		uid := obj.GetUID()
		if uid != "" {
			uids.Insert(string(uid))
		}
	}
	return uids
}

// ResourceGeneration looks up the generation of the given resource
// after it was applied.
func (tc *TaskContext) ResourceGeneration(id object.ObjMetadata) (int64, bool) {
	obj, found := tc.appliedResources[id]
	if !found {
		return 0, false
	}
	return obj.GetGeneration(), true
}

func (tc *TaskContext) ResourceFailed(id object.ObjMetadata) bool {
	_, found := tc.failedResources[id]
	return found
}

func (tc *TaskContext) CaptureResourceFailure(id object.ObjMetadata) {
	tc.failedResources[id] = struct{}{}
}

func (tc *TaskContext) ResourceFailures() []object.ObjMetadata {
	failures := make([]object.ObjMetadata, 0, len(tc.failedResources))
	for f := range tc.failedResources {
		failures = append(failures, f)
	}
	return failures
}

func (tc *TaskContext) CapturePruneFailure(id object.ObjMetadata) {
	tc.pruneFailures[id] = struct{}{}
}

func (tc *TaskContext) PruneFailures() []object.ObjMetadata {
	failures := make([]object.ObjMetadata, 0, len(tc.pruneFailures))
	for f := range tc.pruneFailures {
		failures = append(failures, f)
	}
	return failures
}

// PruneFailed returns true if the passed object identifier
// has been stored as a prune failure; false otherwise.
func (tc *TaskContext) PruneFailed(id object.ObjMetadata) bool {
	_, found := tc.pruneFailures[id]
	return found
}

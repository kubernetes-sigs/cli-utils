// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// NewTaskContext returns a new TaskContext
func NewTaskContext(eventChannel chan event.Event, resourceCache cache.ResourceCache) *TaskContext {
	return &TaskContext{
		taskChannel:       make(chan TaskResult),
		eventChannel:      eventChannel,
		resourceCache:     resourceCache,
		successfulApplies: make(map[object.ObjMetadata]applyInfo),
		failedApplies:     make(map[object.ObjMetadata]struct{}),
		failedDeletes:     make(map[object.ObjMetadata]struct{}),
		skippedApplies:    make(map[object.ObjMetadata]struct{}),
		skippedDeletes:    make(map[object.ObjMetadata]struct{}),
		abandonedObjects:  make(map[object.ObjMetadata]struct{}),
	}
}

// TaskContext defines a context that is passed between all
// the tasks that is in a taskqueue.
type TaskContext struct {
	taskChannel       chan TaskResult
	eventChannel      chan event.Event
	resourceCache     cache.ResourceCache
	successfulApplies map[object.ObjMetadata]applyInfo
	failedApplies     map[object.ObjMetadata]struct{}
	failedDeletes     map[object.ObjMetadata]struct{}
	skippedApplies    map[object.ObjMetadata]struct{}
	skippedDeletes    map[object.ObjMetadata]struct{}
	abandonedObjects  map[object.ObjMetadata]struct{}
}

func (tc *TaskContext) TaskChannel() chan TaskResult {
	return tc.taskChannel
}

func (tc *TaskContext) EventChannel() chan event.Event {
	return tc.eventChannel
}

func (tc *TaskContext) ResourceCache() cache.ResourceCache {
	return tc.resourceCache
}

// SendEvent sends an event on the event channel
func (tc *TaskContext) SendEvent(e event.Event) {
	klog.V(5).Infof("sending event: %s", e)
	tc.eventChannel <- e
}

// IsSuccessfulApply returns true if the object apply was successful
func (tc *TaskContext) IsSuccessfulApply(id object.ObjMetadata) bool {
	_, found := tc.successfulApplies[id]
	return found
}

// AddSuccessfulApply updates the context with information about the
// resource identified by the provided id. Currently, we keep information
// about the generation of the resource after the apply operation completed.
func (tc *TaskContext) AddSuccessfulApply(id object.ObjMetadata, uid types.UID, gen int64) {
	tc.successfulApplies[id] = applyInfo{
		generation: gen,
		uid:        uid,
	}
}

// SuccessfulApplies returns all the objects (as ObjMetadata) that
// were added as applied resources to the TaskContext.
func (tc *TaskContext) SuccessfulApplies() object.ObjMetadataSet {
	all := make(object.ObjMetadataSet, 0, len(tc.successfulApplies))
	for r := range tc.successfulApplies {
		all = append(all, r)
	}
	return all
}

// AppliedResourceUID looks up the UID of the given resource
func (tc *TaskContext) AppliedResourceUID(id object.ObjMetadata) (types.UID, bool) {
	ai, found := tc.successfulApplies[id]
	if klog.V(4).Enabled() {
		if found {
			klog.Infof("resource applied UID cache hit (%s): %d", id, ai.uid)
		} else {
			klog.Infof("resource applied UID cache miss: (%s): %d", id, ai.uid)
		}
	}
	if !found {
		return "", false
	}
	return ai.uid, true
}

// AppliedResourceUIDs returns a set with the UIDs of all the
// successfully applied resources.
func (tc *TaskContext) AppliedResourceUIDs() sets.String {
	uids := sets.NewString()
	for _, ai := range tc.successfulApplies {
		uid := string(ai.uid)
		if uid != "" {
			uids.Insert(uid)
		}
	}
	return uids
}

// AppliedGeneration looks up the generation of the given resource
// after it was applied.
func (tc *TaskContext) AppliedGeneration(id object.ObjMetadata) (int64, bool) {
	ai, found := tc.successfulApplies[id]
	if klog.V(4).Enabled() {
		if found {
			klog.Infof("resource applied generation cache hit (%s): %d", id, ai.generation)
		} else {
			klog.Infof("resource applied generation cache miss: (%s): %d", id, ai.generation)
		}
	}
	if !found {
		return 0, false
	}
	return ai.generation, true
}

// IsFailedApply returns true if the object failed to apply
func (tc *TaskContext) IsFailedApply(id object.ObjMetadata) bool {
	_, found := tc.failedApplies[id]
	return found
}

// AddFailedApply registers that the object failed to apply
func (tc *TaskContext) AddFailedApply(id object.ObjMetadata) {
	tc.failedApplies[id] = struct{}{}
}

// FailedApplies returns all the objects that failed to apply
func (tc *TaskContext) FailedApplies() object.ObjMetadataSet {
	return object.ObjMetadataSetFromMap(tc.failedApplies)
}

// IsFailedDelete returns true if the object failed to delete
func (tc *TaskContext) IsFailedDelete(id object.ObjMetadata) bool {
	_, found := tc.failedDeletes[id]
	return found
}

// AddFailedDelete registers that the object failed to delete
func (tc *TaskContext) AddFailedDelete(id object.ObjMetadata) {
	tc.failedDeletes[id] = struct{}{}
}

// FailedDeletes returns all the objects that failed to delete
func (tc *TaskContext) FailedDeletes() object.ObjMetadataSet {
	return object.ObjMetadataSetFromMap(tc.failedDeletes)
}

// IsSkippedApply returns true if the object apply was skipped
func (tc *TaskContext) IsSkippedApply(id object.ObjMetadata) bool {
	_, found := tc.skippedApplies[id]
	return found
}

// AddSkippedApply registers that the object apply was skipped
func (tc *TaskContext) AddSkippedApply(id object.ObjMetadata) {
	tc.skippedApplies[id] = struct{}{}
}

// SkippedApplies returns all the objects where apply was skipped
func (tc *TaskContext) SkippedApplies() object.ObjMetadataSet {
	return object.ObjMetadataSetFromMap(tc.skippedApplies)
}

// IsSkippedDelete returns true if the object delete was skipped
func (tc *TaskContext) IsSkippedDelete(id object.ObjMetadata) bool {
	_, found := tc.skippedDeletes[id]
	return found
}

// AddSkippedDelete registers that the object delete was skipped
func (tc *TaskContext) AddSkippedDelete(id object.ObjMetadata) {
	tc.skippedDeletes[id] = struct{}{}
}

// SkippedDeletes returns all the objects where deletion was skipped
func (tc *TaskContext) SkippedDeletes() object.ObjMetadataSet {
	return object.ObjMetadataSetFromMap(tc.skippedDeletes)
}

// IsAbandonedObject returns true if the object is abandoned
func (tc *TaskContext) IsAbandonedObject(id object.ObjMetadata) bool {
	_, found := tc.abandonedObjects[id]
	return found
}

// AddAbandonedObject registers that the object is abandoned
func (tc *TaskContext) AddAbandonedObject(id object.ObjMetadata) {
	tc.abandonedObjects[id] = struct{}{}
}

// AbandonedObjects returns all the abandoned objects
func (tc *TaskContext) AbandonedObjects() object.ObjMetadataSet {
	return object.ObjMetadataSetFromMap(tc.abandonedObjects)
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

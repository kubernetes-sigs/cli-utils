// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var (
	crdGK = schema.GroupKind{Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition"}
)

// Task is the interface that must be implemented by
// all tasks that will be executed by the taskrunner.
type Task interface {
	Name() string
	Action() event.ResourceAction
	Identifiers() object.ObjMetadataSet
	Start(*TaskContext)
	StatusUpdate(*TaskContext, object.ObjMetadata)
	Cancel(*TaskContext)
}

// NewWaitTask creates a new wait task where we will wait until
// the resources specifies by ids all meet the specified condition.
func NewWaitTask(name string, ids object.ObjMetadataSet, cond Condition, timeout time.Duration, mapper meta.RESTMapper) *WaitTask {
	return &WaitTask{
		name:      name,
		Ids:       ids,
		Condition: cond,
		Timeout:   timeout,
		mapper:    mapper,
	}
}

// WaitTask is an implementation of the Task interface that is used
// to wait for a set of resources (identified by a slice of ObjMetadata)
// will all meet the condition specified. It also specifies a timeout
// for how long we are willing to wait for this to happen.
// Unlike other implementations of the Task interface, the wait task
// is handled in a special way to the taskrunner and is a part of the core
// package.
type WaitTask struct {
	// name allows providing a name for the task.
	name string
	// Ids is the list of resources that we are waiting for.
	Ids object.ObjMetadataSet
	// Condition defines the status we want all resources to reach
	Condition Condition
	// Timeout defines how long we are willing to wait for the condition
	// to be met.
	Timeout time.Duration

	mapper meta.RESTMapper

	// cancelFunc is a function that will cancel the timeout timer
	// on the task.
	cancelFunc func()
}

func (w *WaitTask) Name() string {
	return w.name
}

func (w *WaitTask) Action() event.ResourceAction {
	return event.WaitAction
}

func (w *WaitTask) Identifiers() object.ObjMetadataSet {
	return w.Ids
}

// Start kicks off the task. For the wait task, this just means
// setting up the timeout timer.
func (w *WaitTask) Start(taskContext *TaskContext) {
	klog.V(2).Infof("starting wait task (name: %q, objects: %d)", w.Name(), len(w.Ids))

	// TODO: inherit context from task runner, passed through the TaskContext
	ctx := context.Background()

	// use a context wrapper to handle complete/cancel/timeout
	if w.Timeout > 0 {
		ctx, w.cancelFunc = context.WithTimeout(ctx, w.Timeout)
	} else {
		ctx, w.cancelFunc = context.WithCancel(ctx)
	}

	// A goroutine to handle ending the WaitTask.
	go func() {
		// Block until complete/cancel/timeout
		<-ctx.Done()
		// Err is always non-nil when Done channel is closed.
		err := ctx.Err()

		klog.V(2).Infof("completing wait task (name: %q)", w.name)

		// reset RESTMapper, if a CRD was applied/pruned
		foundCRD := false
		for _, obj := range w.Ids {
			if obj.GroupKind == crdGK {
				foundCRD = true
				break
			}
		}
		if foundCRD {
			w.resetRESTMapper()
		}

		switch err {
		case context.Canceled:
			// happy path - cancelled or completed (not considered an error)
		case context.DeadlineExceeded:
			// timed out
			w.sendTimeoutEvents(taskContext)
		default:
			// shouldn't happen, per context docs
			klog.Errorf("wait task stopped with unexpected context error: %v", err)
		}

		// Done here. signal completion to the task runner
		taskContext.TaskChannel() <- TaskResult{}
	}()

	// send initial events for all resources being waited on
	for _, id := range w.Ids {
		switch {
		case w.skipped(taskContext, id):
			w.sendEvent(taskContext, id, event.ReconcileSkipped)
		case w.reconciledByID(taskContext, id):
			w.sendEvent(taskContext, id, event.Reconciled)
		default:
			w.sendEvent(taskContext, id, event.ReconcilePending)
		}
	}

	// exit early if all conditions are met
	if w.reconciled(taskContext) {
		w.cancelFunc()
	}
}

func (w *WaitTask) sendEvent(taskContext *TaskContext, id object.ObjMetadata, op event.WaitEventOperation) {
	taskContext.EventChannel() <- event.Event{
		Type: event.WaitType,
		WaitEvent: event.WaitEvent{
			GroupName:  w.Name(),
			Identifier: id,
			Operation:  op,
		},
	}
}

// sendTimeoutEvents sends a timeout event for every pending object that isn't
// reconciled.
func (w *WaitTask) sendTimeoutEvents(taskContext *TaskContext) {
	for _, id := range w.pending(taskContext) {
		if !w.reconciledByID(taskContext, id) {
			w.sendEvent(taskContext, id, event.ReconcileTimeout)
		}
	}
}

// reconciledByID checks whether the condition set in the task is currently met
// for the specified object given the status of resource in the cache.
func (w *WaitTask) reconciledByID(taskContext *TaskContext, id object.ObjMetadata) bool {
	return conditionMet(taskContext, object.ObjMetadataSet{id}, w.Condition)
}

// reconciled checks whether the condition set in the task is currently met
// given the status of resources in the cache.
func (w *WaitTask) reconciled(taskContext *TaskContext) bool {
	return conditionMet(taskContext, w.pending(taskContext), w.Condition)
}

// pending returns the set of resources being waited on (not skipped).
func (w *WaitTask) pending(taskContext *TaskContext) object.ObjMetadataSet {
	var ids object.ObjMetadataSet
	for _, id := range w.Ids {
		if !w.skipped(taskContext, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// skipped returns true if the object failed or was skipped by a preceding
// apply/delete/prune task.
func (w *WaitTask) skipped(taskContext *TaskContext, id object.ObjMetadata) bool {
	if w.Condition == AllCurrent &&
		taskContext.IsFailedApply(id) || taskContext.IsSkippedApply(id) {
		return true
	}
	if w.Condition == AllNotFound &&
		taskContext.IsFailedDelete(id) || taskContext.IsSkippedDelete(id) {
		return true
	}
	return false
}

// Cancel exits early with a timeout error
func (w *WaitTask) Cancel(_ *TaskContext) {
	w.cancelFunc()
}

// StatusUpdate validates whether the update meets the conditions to stop
// the wait task. If the status is for a watched object and that object now
// meets the desired condition, a WaitEvent will be sent before exiting.
func (w *WaitTask) StatusUpdate(taskContext *TaskContext, id object.ObjMetadata) {
	if klog.V(5).Enabled() {
		status := taskContext.ResourceCache().Get(id).Status
		klog.Errorf("status update (object: %q, status: %q)", id, status)
	}

	// ignored objects have already had skipped events sent at start
	if w.skipped(taskContext, id) {
		return
	}

	// if the condition is met for this object, send a wait event
	if w.reconciledByID(taskContext, id) {
		taskContext.EventChannel() <- event.Event{
			Type: event.WaitType,
			WaitEvent: event.WaitEvent{
				GroupName:  w.Name(),
				Identifier: id,
				Operation:  event.Reconciled,
			},
		}
	}

	// if all conditions are met, complete the wait task
	if w.reconciled(taskContext) {
		w.cancelFunc()
	}
}

// resetRESTMapper resets the RESTMapper so it can pick up new CRDs.
func (w *WaitTask) resetRESTMapper() {
	// TODO: find a way to add/remove mappers without resetting the entire mapper
	// Resetting the mapper requires all CRDs to be queried again.
	ddRESTMapper, err := extractDeferredDiscoveryRESTMapper(w.mapper)
	if err != nil {
		if klog.V(4).Enabled() {
			klog.Errorf("error resetting RESTMapper: %v", err)
		}
	}
	ddRESTMapper.Reset()
}

// extractDeferredDiscoveryRESTMapper unwraps the provided RESTMapper
// interface to get access to the underlying DeferredDiscoveryRESTMapper
// that can be reset.
func extractDeferredDiscoveryRESTMapper(mapper meta.RESTMapper) (*restmapper.DeferredDiscoveryRESTMapper,
	error) {
	val := reflect.ValueOf(mapper)
	if val.Type().Kind() != reflect.Struct {
		return nil, fmt.Errorf("unexpected RESTMapper type: %s", val.Type().String())
	}
	fv := val.FieldByName("RESTMapper")
	ddRESTMapper, ok := fv.Interface().(*restmapper.DeferredDiscoveryRESTMapper)
	if !ok {
		return nil, fmt.Errorf("unexpected RESTMapper type")
	}
	return ddRESTMapper, nil
}

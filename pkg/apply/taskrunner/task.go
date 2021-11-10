// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"context"
	"fmt"
	"reflect"
	"sync"
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
	// Ids is the full list of resources that we are waiting for.
	Ids object.ObjMetadataSet
	// Condition defines the status we want all resources to reach
	Condition Condition
	// Timeout defines how long we are willing to wait for the condition
	// to be met.
	Timeout time.Duration
	// mapper is the RESTMapper to update after CRDs have been reconciled
	mapper meta.RESTMapper
	// cancelFunc is a function that will cancel the timeout timer
	// on the task.
	cancelFunc context.CancelFunc
	// pending is the set of resources that we are still waiting for.
	pending object.ObjMetadataSet
	// mu protects the pending ObjMetadataSet
	mu sync.RWMutex
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
	klog.V(2).Infof("wait task starting (name: %q, objects: %d)",
		w.Name(), len(w.Ids))

	// TODO: inherit context from task runner, passed through the TaskContext
	ctx := context.Background()

	// use a context wrapper to handle complete/cancel/timeout
	if w.Timeout > 0 {
		ctx, w.cancelFunc = context.WithTimeout(ctx, w.Timeout)
	} else {
		ctx, w.cancelFunc = context.WithCancel(ctx)
	}

	w.startInner(taskContext)

	// A goroutine to handle ending the WaitTask.
	go func() {
		// Block until complete/cancel/timeout
		<-ctx.Done()
		// Err is always non-nil when Done channel is closed.
		err := ctx.Err()

		klog.V(2).Infof("wait task completing (name: %q,): %v", w.name, err)

		switch err {
		case context.Canceled:
			// happy path - cancelled or completed (not considered an error)
		case context.DeadlineExceeded:
			// timed out
			w.sendTimeoutEvents(taskContext)
		}

		// Update RESTMapper to pick up new custom resource types
		w.updateRESTMapper(taskContext)

		// Done here. signal completion to the task runner
		taskContext.TaskChannel() <- TaskResult{}
	}()
}

func (w *WaitTask) sendEvent(taskContext *TaskContext, id object.ObjMetadata, op event.WaitEventOperation) {
	taskContext.SendEvent(event.Event{
		Type: event.WaitType,
		WaitEvent: event.WaitEvent{
			GroupName:  w.Name(),
			Identifier: id,
			Operation:  op,
		},
	})
}

// startInner sends initial pending, skipped, an reconciled events.
// If all objects are reconciled or skipped, cancelFunc is called.
// The pending set is write locked during execution of startInner.
func (w *WaitTask) startInner(taskContext *TaskContext) {
	w.mu.Lock()
	defer w.mu.Unlock()

	pending := object.ObjMetadataSet{}
	for _, id := range w.Ids {
		switch {
		case w.skipped(taskContext, id):
			w.sendEvent(taskContext, id, event.ReconcileSkipped)
		case w.reconciledByID(taskContext, id):
			w.sendEvent(taskContext, id, event.Reconciled)
		default:
			pending = append(pending, id)
			w.sendEvent(taskContext, id, event.ReconcilePending)
		}
	}
	w.pending = pending

	if len(pending) == 0 {
		// all reconciled - clear pending and exit
		klog.V(3).Infof("all objects reconciled or skipped (name: %q)", w.name)
		w.cancelFunc()
	}
}

// sendTimeoutEvents sends a timeout event for every remaining pending object
// The pending set is read locked during execution of sendTimeoutEvents.
func (w *WaitTask) sendTimeoutEvents(taskContext *TaskContext) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, id := range w.pending {
		w.sendEvent(taskContext, id, event.ReconcileTimeout)
	}
}

// reconciledByID checks whether the condition set in the task is currently met
// for the specified object given the status of resource in the cache.
func (w *WaitTask) reconciledByID(taskContext *TaskContext, id object.ObjMetadata) bool {
	return conditionMet(taskContext, object.ObjMetadataSet{id}, w.Condition)
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

// StatusUpdate records objects status updates and sends WaitEvents.
// If all objects are reconciled or skipped, cancelFunc is called.
// The pending set is write locked during execution of StatusUpdate.
func (w *WaitTask) StatusUpdate(taskContext *TaskContext, id object.ObjMetadata) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if klog.V(5).Enabled() {
		status := taskContext.ResourceCache().Get(id).Status
		klog.Errorf("status update (object: %q, status: %q)", id, status)
	}

	switch {
	case w.pending.Contains(id):
		// pending - check if reconciled
		if w.reconciledByID(taskContext, id) {
			// reconciled - remove from pending & send event
			w.pending = w.pending.Remove(id)
			w.sendEvent(taskContext, id, event.Reconciled)
		} else {
			// can't be all reconciled now, so don't bother checking
			return
		}
	case !w.Ids.Contains(id):
		// not in wait group - ignore
		return
	case w.skipped(taskContext, id):
		// skipped - ignore
		return
	default:
		// reconciled - check if unreconciled
		if !w.reconciledByID(taskContext, id) {
			// unreconciled - add to pending & send event
			w.pending = append(w.pending, id)
			w.sendEvent(taskContext, id, event.ReconcilePending)
			// can't be all reconciled now, so don't bother checking
			return
		}
	}

	// if all conditions are met, complete the wait task
	if conditionMet(taskContext, w.pending, w.Condition) {
		// all reconciled - clear pending and exit
		w.pending = object.ObjMetadataSet{}
		klog.V(3).Infof("all objects reconciled or skipped (name: %q)", w.name)
		w.cancelFunc()
	}
}

// updateRESTMapper resets the RESTMapper if CRDs were applied, so that new
// resource types can be applied by subsequent tasks.
// TODO: find a way to add/remove mappers without resetting the entire mapper
// Resetting the mapper requires all CRDs to be queried again.
func (w *WaitTask) updateRESTMapper(taskContext *TaskContext) {
	foundCRD := false
	for _, id := range w.Ids {
		if id.GroupKind == crdGK && !w.skipped(taskContext, id) {
			foundCRD = true
			break
		}
	}
	if !foundCRD {
		// no update required
		return
	}

	klog.V(5).Infof("resetting RESTMapper")
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
func extractDeferredDiscoveryRESTMapper(mapper meta.RESTMapper) (
	*restmapper.DeferredDiscoveryRESTMapper,
	error,
) {
	val := reflect.ValueOf(mapper)
	if val.Type().Kind() != reflect.Struct {
		return nil, fmt.Errorf("unexpected RESTMapper type: %s", val.Type().String())
	}
	fv := val.FieldByName("RESTMapper")
	ddRESTMapper, ok := fv.Interface().(*restmapper.DeferredDiscoveryRESTMapper)
	if !ok {
		return nil, fmt.Errorf("unexpected RESTMapper field type: %s", fv.Type())
	}
	return ddRESTMapper, nil
}

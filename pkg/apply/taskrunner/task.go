// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Task is the interface that must be implemented by
// all tasks that will be executed by the taskrunner.
type Task interface {
	Name() string
	Action() event.ResourceAction
	Identifiers() []object.ObjMetadata
	Start(*TaskContext)
	OnStatusEvent(*TaskContext, event.StatusEvent)
}

// NewWaitTask creates a new wait task where we will wait until
// the resources specifies by ids all meet the specified condition.
func NewWaitTask(name string, ids []object.ObjMetadata, cond Condition, timeout time.Duration, mapper meta.RESTMapper) *WaitTask {
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
	Ids []object.ObjMetadata
	// Condition defines the status we want all resources to reach
	Condition Condition
	// Timeout defines how long we are willing to wait for the condition
	// to be met.
	Timeout time.Duration

	mapper meta.RESTMapper

	// once ensures errorCh only receives one error
	once    sync.Once
	errorCh chan error
}

func (w *WaitTask) Name() string {
	return w.name
}

func (w *WaitTask) Action() event.ResourceAction {
	return event.WaitAction
}

func (w *WaitTask) Identifiers() []object.ObjMetadata {
	return w.Ids
}

// Start kicks off the task. For the wait task, this just means
// setting up the timeout timer.
func (w *WaitTask) Start(taskContext *TaskContext) {
	klog.V(2).Infof("starting wait task (%d objects)", len(w.Ids))

	// reset task
	w.once = sync.Once{}
	w.errorCh = make(chan error)

	// WaitTask gets its own context to diferentiate between WaitTask timeout
	// and TaskQueue timeout. First timeout wins!
	taskCtx := context.Background()
	var taskCancel func()
	if w.Timeout != 0 {
		klog.V(5).Infof("wait task timeout: %s", w.Timeout)
		taskCtx, taskCancel = context.WithTimeout(taskCtx, w.Timeout)
	} else {
		taskCtx, taskCancel = context.WithCancel(taskCtx)
	}

	// Wrap the parent context to ensure it's done.
	ctx, cancel := context.WithCancel(taskContext.Context())

	// wait until parent timeout or cancel
	go func() {
		defer func() {
			// cancel contexts to free up resources
			taskCancel()
			cancel()
		}()

		<-ctx.Done()
		klog.V(3).Info("wait task parent context done")
		w.stop(ctx.Err())
	}()

	// wait until task timeout or cancel
	go func() {
		defer func() {
			// cancel contexts to free up resources
			taskCancel()
			cancel()
		}()

		<-taskCtx.Done()
		klog.V(3).Info("wait task context done")
		w.stop(w.unwrapTaskTimeout(taskCtx.Err()))
	}()

	// wait until complete (optional error on errorCh)
	go func() {
		defer func() {
			// cancel contexts to free up resources
			taskCancel()
			cancel()
		}()

		err := <-w.errorCh
		klog.V(3).Info("wait task completed")
		taskContext.TaskChannel() <- TaskResult{
			Err: err,
		}
	}()

	if w.conditionMet(taskContext) {
		klog.V(3).Info("wait condition met, stopping task early")
		w.stop(nil)
	}
}

func (w *WaitTask) OnStatusEvent(taskContext *TaskContext, _ event.StatusEvent) {
	if w.conditionMet(taskContext) {
		klog.V(3).Info("wait condition met, stopping task")
		w.stop(nil)
	}
}

// conditionMet checks whether the condition set in the task
// is currently met given the status of resources in the collector.
func (w *WaitTask) conditionMet(taskContext *TaskContext) bool {
	rwd := w.resourcesToWaitFor(taskContext)
	return taskContext.ResourceStatusCollector().ConditionMet(rwd, w.Condition)
}

// resourcesToWaitFor creates a slice of ResourceGeneration for
// the resources that is relevant to this wait task. The objective is
// to match each resource with the generation seen after the resource
// was applied.
func (w *WaitTask) resourcesToWaitFor(taskContext *TaskContext) []ResourceGeneration {
	var rwd []ResourceGeneration
	for _, id := range w.Ids {
		// Skip checking condition for resources which have failed
		// to apply or failed to prune/delete (depending on wait condition).
		// This includes resources which are skipped because of filtering.
		if (w.Condition == AllCurrent && taskContext.ResourceFailed(id)) ||
			(w.Condition == AllNotFound && taskContext.PruneFailed(id)) {
			continue
		}
		gen, _ := taskContext.ResourceGeneration(id)
		rwd = append(rwd, ResourceGeneration{
			Identifier: id,
			Generation: gen,
		})
	}
	return rwd
}

// stop resets the RESTMapper if any Ids are CRDs and sends the error to the
// errorCh, once per task start.
func (w *WaitTask) stop(err error) {
	w.once.Do(func() {
		klog.V(3).Info("wait task complete")
		for _, obj := range w.Ids {
			if (obj.GroupKind.Group == v1.SchemeGroupVersion.Group ||
				obj.GroupKind.Group == v1beta1.SchemeGroupVersion.Group) &&
				obj.GroupKind.Kind == "CustomResourceDefinition" {
				ddRESTMapper, err := extractDeferredDiscoveryRESTMapper(w.mapper)
				if err == nil {
					ddRESTMapper.Reset()
					// We only need to reset once.
					break
				}
			}
		}
		w.errorCh <- err
	})
}

func (w *WaitTask) unwrapTaskTimeout(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		err = &TimeoutError{
			Identifiers: w.Ids,
			Timeout:     w.Timeout,
			Condition:   w.Condition,
		}
	}
	return err
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

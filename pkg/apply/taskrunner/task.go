// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"fmt"
	"reflect"
	"time"

	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Task is the interface that must be implemented by
// all tasks that will be executed by the taskrunner.
type Task interface {
	Name() string
	Action() event.ResourceAction
	Identifiers() []object.ObjMetadata
	Start(taskContext *TaskContext)
	ClearTimeout()
}

// NewWaitTask creates a new wait task where we will wait until
// the resources specifies by ids all meet the specified condition.
func NewWaitTask(name string, ids []object.ObjMetadata, cond Condition, timeout time.Duration, mapper meta.RESTMapper) *WaitTask {
	// Create the token channel and only add one item.
	tokenChannel := make(chan struct{}, 1)
	tokenChannel <- struct{}{}

	return &WaitTask{
		name:      name,
		Ids:       ids,
		Condition: cond,
		Timeout:   timeout,

		mapper: mapper,
		token:  tokenChannel,
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

	// cancelFunc is a function that will cancel the timeout timer
	// on the task.
	cancelFunc func()

	// token is a channel that is provided a single item when the
	// task is created. Goroutines are only allowed to write to the
	// taskChannel if they are able to get the item from the channel.
	// This makes sure that the task only results in one message on the
	// taskChannel, even if the condition is met and the task times out
	// at the same time.
	token chan struct{}
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
	w.setTimer(taskContext)
}

// setTimer creates the timer with the timeout value taken from
// the WaitTask struct. Once the timer expires, it will send
// a message on the TaskChannel provided in the taskContext.
func (w *WaitTask) setTimer(taskContext *TaskContext) {
	timer := time.NewTimer(w.Timeout)
	go func() {
		// TODO(mortent): See if there is a better way to do this. This
		// solution will cause the goroutine to hang forever if the
		// Timeout is cancelled.
		<-timer.C
		select {
		// We only send the taskResult if no one has gotten
		// to the token first.
		case <-w.token:
			taskContext.TaskChannel() <- TaskResult{
				Err: &TimeoutError{
					Identifiers: w.Ids,
					Timeout:     w.Timeout,
					Condition:   w.Condition,
				},
			}
		default:
			return
		}
	}()
	w.cancelFunc = func() {
		timer.Stop()
	}
}

// checkCondition checks whether the condition set in the task
// is currently met given the status of resources in the collector.
func (w *WaitTask) checkCondition(taskContext *TaskContext, coll *resourceStatusCollector) bool {
	rwd := w.computeResourceWaitData(taskContext)
	return coll.conditionMet(rwd, w.Condition)
}

// computeResourceWaitData creates a slice of resourceWaitData for
// the resources that is relevant to this wait task. The objective is
// to match each resource with the generation seen after the resource
// was applied.
func (w *WaitTask) computeResourceWaitData(taskContext *TaskContext) []resourceWaitData {
	var rwd []resourceWaitData
	for _, id := range w.Ids {
		// Skip checking condition for resources which have failed
		// to apply or failed to prune/delete (depending on wait condition).
		// This includes resources which are skipped because of filtering.
		if (w.Condition == AllCurrent && taskContext.ResourceFailed(id)) ||
			(w.Condition == AllNotFound && taskContext.PruneFailed(id)) {
			continue
		}
		gen, _ := taskContext.ResourceGeneration(id)
		rwd = append(rwd, resourceWaitData{
			identifier: id,
			generation: gen,
		})
	}
	return rwd
}

// startAndComplete is invoked when the condition is already
// met when the task should be started. In this case there is no
// need to start a timer. So it just sets the cancelFunc and then
// completes the task.
func (w *WaitTask) startAndComplete(taskContext *TaskContext) {
	w.cancelFunc = func() {}
	w.complete(taskContext)
}

// complete is invoked by the taskrunner when all the conditions
// for the task has been met, or something has failed so the task
// need to be stopped.
func (w *WaitTask) complete(taskContext *TaskContext) {
	var err error
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
			continue
		}
	}
	select {
	// Only do something if we can get the token.
	case <-w.token:
		go func() {
			taskContext.TaskChannel() <- TaskResult{
				Err: err,
			}
		}()
	default:
		return
	}
}

// ClearTimeout cancels the timeout for the wait task.
func (w *WaitTask) ClearTimeout() {
	w.cancelFunc()
}

type resourceWaitData struct {
	identifier object.ObjMetadata
	generation int64
}

// Condition is a type that defines the types of conditions
// which a WaitTask can use.
type Condition string

const (
	// AllCurrent Condition means all the provided resources
	// has reached (and remains in) the Current status.
	AllCurrent Condition = "AllCurrent"

	// AllNotFound Condition means all the provided resources
	// has reached the NotFound status, i.e. they are all deleted
	// from the cluster.
	AllNotFound Condition = "AllNotFound"
)

// Meets returns true if the provided status meets the condition and
// false if it does not.
func (c Condition) Meets(s status.Status) bool {
	switch c {
	case AllCurrent:
		return s == status.CurrentStatus
	case AllNotFound:
		return s == status.NotFoundStatus
	default:
		return false
	}
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

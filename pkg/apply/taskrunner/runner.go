// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"context"
	"fmt"
	"sort"
	"time"

	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// NewTaskStatusRunner returns a new TaskStatusRunner.
func NewTaskStatusRunner(identifiers object.ObjMetadataSet, statusPoller poller.Poller, cache cache.ResourceCache) *taskStatusRunner {
	return &taskStatusRunner{
		identifiers:  identifiers,
		statusPoller: statusPoller,
		baseRunner:   newBaseRunner(cache),
	}
}

// taskStatusRunner is a taskRunner that executes a set of
// tasks while at the same time uses the statusPoller to
// keep track of the status of the resources.
type taskStatusRunner struct {
	identifiers  object.ObjMetadataSet
	statusPoller poller.Poller

	baseRunner *baseRunner
}

// Options defines properties that is passed along to
// the statusPoller.
type Options struct {
	PollInterval     time.Duration
	UseCache         bool
	EmitStatusEvents bool
}

// Run starts the execution of the taskqueue. It will start the
// statusPoller and then pass the statusChannel to the baseRunner
// that does most of the work.
func (tsr *taskStatusRunner) Run(ctx context.Context, taskQueue chan Task,
	eventChannel chan event.Event, options Options) error {
	statusCtx, cancelFunc := context.WithCancel(context.Background())
	statusChannel := tsr.statusPoller.Poll(statusCtx, tsr.identifiers, polling.Options{
		PollInterval: options.PollInterval,
		UseCache:     options.UseCache,
	})

	o := baseOptions{
		emitStatusEvents: options.EmitStatusEvents,
	}
	err := tsr.baseRunner.run(ctx, taskQueue, statusChannel, eventChannel, o)
	// cancel the statusPoller by cancelling the context.
	cancelFunc()
	// drain the statusChannel to make sure the lack of a consumer
	// doesn't block the shutdown of the statusPoller.
	for range statusChannel {
	}
	return err
}

// NewTaskRunner returns a new taskRunner. It can process taskqueues
// that does not contain any wait tasks.
// TODO: Do we need this abstraction layer now that baseRunner doesn't need a collector?
func NewTaskRunner() *taskRunner {
	return &taskRunner{
		baseRunner: newBaseRunner(cache.NewResourceCacheMap()),
	}
}

// taskRunner is a simplified taskRunner that does not support
// wait tasks and does not provide any status updates for the
// resources. This is useful in situations where we are not interested
// in status, for example during dry-run.
type taskRunner struct {
	baseRunner *baseRunner
}

// Run starts the execution of the task queue. It delegates the
// work to the baseRunner, but gives it as nil channel as the statusChannel.
func (tr *taskRunner) Run(ctx context.Context, taskQueue chan Task,
	eventChannel chan event.Event) error {
	var nilStatusChannel chan pollevent.Event
	o := baseOptions{
		// The taskRunner doesn't poll for status, so there are not
		// statusEvents to emit.
		emitStatusEvents: false,
	}
	return tr.baseRunner.run(ctx, taskQueue, nilStatusChannel, eventChannel, o)
}

// newBaseRunner returns a new baseRunner using the provided cache.
func newBaseRunner(cache cache.ResourceCache) *baseRunner {
	return &baseRunner{
		cache: cache,
	}
}

// baseRunner provides the basic task runner functionality.
//
// The cache can be used by tasks to retrieve the last known resource state.
//
// This is not meant to be used directly. It is used by the taskRunner and
// taskStatusRunner.
type baseRunner struct {
	cache cache.ResourceCache
}

type baseOptions struct {
	// emitStatusEvents enables emitting events on the eventChannel
	emitStatusEvents bool
}

// run executes the tasks in the taskqueue.
//
// The tasks run in a loop where a single goroutine will process events from
// three different channels.
// - taskQueue is read to allow updating the task queue at runtime.
// - statusChannel is read to allow updates to the resource cache and triggering
//   validation of wait conditions.
// - eventChannel is written to with events based on status updates, if
//   emitStatusEvents is true.
func (b *baseRunner) run(ctx context.Context, taskQueue chan Task,
	statusChannel <-chan pollevent.Event, eventChannel chan event.Event,
	o baseOptions) error {
	// taskContext is passed into all tasks when they are started. It
	// provides access to the eventChannel and the taskChannel, and
	// also provides a way to pass data between tasks.
	taskContext := NewTaskContext(eventChannel, b.cache)

	// Find and start the first task in the queue.
	currentTask, done := b.nextTask(taskQueue, taskContext)
	if done {
		return nil
	}

	// abort is used to signal that something has failed, and
	// the task processing should end as soon as is possible. Only
	// wait tasks can be interrupted, so for all other tasks we need
	// to wait for the currently running one to finish before we can
	// exit.
	abort := false
	var abortReason error

	// We do this so we can set the doneCh to a nil channel after
	// it has been closed. This is needed to avoid a busy loop.
	doneCh := ctx.Done()

	for {
		select {
		// This processes status events from a channel, most likely
		// driven by the StatusPoller. All normal resource status update
		// events are passed through to the eventChannel. This means
		// that listeners of the eventChannel will get updates on status
		// even while other tasks (like apply tasks) are running.
		case statusEvent, ok := <-statusChannel:
			// If the statusChannel has closed or we are preparing
			// to abort the task processing, we just ignore all
			// statusEvents.
			// TODO(mortent): Check if a closed statusChannel might
			// create a busy loop here.
			if !ok || abort {
				continue
			}

			// An error event on the statusChannel means the StatusPoller
			// has encountered a problem so it can't continue. This means
			// the statusChannel will be closed soon.
			if statusEvent.EventType == pollevent.ErrorEvent {
				abort = true
				abortReason = fmt.Errorf("polling for status failed: %v",
					statusEvent.Error)
				// If the current task is a wait task, we just set it
				// to complete so we can exit the loop as soon as possible.
				completeIfWaitTask(currentTask, taskContext)
				continue
			}

			if o.emitStatusEvents {
				// Forward all normal events to the eventChannel
				eventChannel <- event.Event{
					Type: event.StatusType,
					StatusEvent: event.StatusEvent{
						Identifier:       statusEvent.Resource.Identifier,
						PollResourceInfo: statusEvent.Resource,
						Resource:         statusEvent.Resource.Resource,
						Error:            statusEvent.Error,
					},
				}
			}

			// Update the cache to track the latest resource spec & status.
			// Status is computed from the resource on-demand.
			// Warning: Resource may be nil!
			taskContext.ResourceCache().Put(
				statusEvent.Resource.Identifier,
				cache.ResourceStatus{
					Resource:      statusEvent.Resource.Resource,
					Status:        statusEvent.Resource.Status,
					StatusMessage: statusEvent.Resource.Message,
				},
			)

			// If the current task is a wait task, we check whether
			// the condition has been met. If so, we complete the task.
			if wt, ok := currentTask.(*WaitTask); ok {
				if wt.checkCondition(taskContext) {
					completeIfWaitTask(currentTask, taskContext)
				}
			}
		// A message on the taskChannel means that the current task
		// has either completed or failed. If it has failed, we return
		// the error. If the abort flag is true, which means something
		// else has gone wrong and we are waiting for the current task to
		// finish, we exit.
		// If everything is ok, we fetch and start the next task.
		case msg := <-taskContext.TaskChannel():
			currentTask.ClearTimeout()
			taskContext.EventChannel() <- event.Event{
				Type: event.ActionGroupType,
				ActionGroupEvent: event.ActionGroupEvent{
					GroupName: currentTask.Name(),
					Action:    currentTask.Action(),
					Type:      event.Finished,
				},
			}
			if msg.Err != nil {
				b.amendTimeoutError(taskContext, msg.Err)
				return msg.Err
			}
			if abort {
				return abortReason
			}
			currentTask, done = b.nextTask(taskQueue, taskContext)
			// If there are no more tasks, we are done. So just
			// return.
			if done {
				return nil
			}
		// The doneCh will be closed if the passed in context is cancelled.
		// If so, we just set the abort flag and wait for the currently running
		// task to complete before we exit.
		case <-doneCh:
			doneCh = nil // Set doneCh to nil so we don't enter a busy loop.
			abort = true
			completeIfWaitTask(currentTask, taskContext)
		}
	}
}

func (b *baseRunner) amendTimeoutError(taskContext *TaskContext, err error) {
	if timeoutErr, ok := err.(*TimeoutError); ok {
		var timedOutResources []TimedOutResource
		for _, id := range timeoutErr.Identifiers {
			result := taskContext.ResourceCache().Get(id)
			if timeoutErr.Condition.Meets(result.Status) {
				continue
			}
			timedOutResources = append(timedOutResources, TimedOutResource{
				Identifier: id,
				Status:     result.Status,
				Message:    result.StatusMessage,
			})
		}
		timeoutErr.TimedOutResources = timedOutResources
	}
}

// completeIfWaitTask checks if the current task is a wait task. If so,
// we invoke the complete function to complete it.
func completeIfWaitTask(currentTask Task, taskContext *TaskContext) {
	if wt, ok := currentTask.(*WaitTask); ok {
		wt.complete(taskContext)
	}
}

// nextTask fetches the latest task from the taskQueue and
// starts it. If the taskQueue is empty, it the second
// return value will be true.
func (b *baseRunner) nextTask(taskQueue chan Task,
	taskContext *TaskContext) (Task, bool) {
	var tsk Task
	select {
	// If there is any tasks left in the queue, this
	// case statement will be executed.
	case t := <-taskQueue:
		tsk = t
	default:
		// Only happens when the channel is empty.
		return nil, true
	}

	taskContext.EventChannel() <- event.Event{
		Type: event.ActionGroupType,
		ActionGroupEvent: event.ActionGroupEvent{
			GroupName: tsk.Name(),
			Action:    tsk.Action(),
			Type:      event.Started,
		},
	}

	switch st := tsk.(type) {
	case *WaitTask:
		// The wait tasks need to be handled specifically here. Before
		// starting a new wait task, we check if the condition is already
		// met. Without this check, a task might end up waiting for
		// status events when the condition is in fact already met.
		if st.checkCondition(taskContext) {
			st.startAndComplete(taskContext)
		} else {
			st.Start(taskContext)
		}
	default:
		tsk.Start(taskContext)
	}
	return tsk, false
}

// TaskResult is the type returned from tasks once they have completed
// or failed. If it has failed or timed out, the Err property will be
// set.
type TaskResult struct {
	Err error
}

// TimeoutError is a special error used by tasks when they have
// timed out.
type TimeoutError struct {
	// Identifiers contains the identifiers of all resources that the
	// WaitTask was waiting for.
	Identifiers object.ObjMetadataSet

	// Timeout is the amount of time it took before it timed out.
	Timeout time.Duration

	// Condition defines the criteria for which the task was waiting.
	Condition Condition

	TimedOutResources []TimedOutResource
}

type TimedOutResource struct {
	Identifier object.ObjMetadata

	Status status.Status

	Message string
}

func (te TimeoutError) Error() string {
	ids := []string{}
	for _, id := range te.Identifiers {
		ids = append(ids, id.String())
	}
	sort.Strings(ids)
	return fmt.Sprintf("timeout after %.0f seconds waiting for %d resources (%v) to reach condition %s",
		te.Timeout.Seconds(), len(te.Identifiers), ids, te.Condition)
}

// IsTimeoutError checks whether a given error is
// a TimeoutError.
func IsTimeoutError(err error) (*TimeoutError, bool) {
	if e, ok := err.(*TimeoutError); ok {
		return e, true
	}
	return &TimeoutError{}, false
}

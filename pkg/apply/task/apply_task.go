// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
)

// ApplyTask applies the given Objects to the cluster
// by using the ApplyOptions.
type ApplyTask struct {
	ApplyOptions *apply.ApplyOptions
	Objects      []*resource.Info
}

// Start creates a new goroutine that will invoke
// the Run function on the ApplyOptions to update
// the cluster. It will push a TaskResult on the taskChannel
// to signal to the taskrunner that the task has completed (or failed).
func (a *ApplyTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		a.ApplyOptions.SetObjects(a.Objects)
		err := a.ApplyOptions.Run()
		taskContext.TaskChannel() <- taskrunner.TaskResult{
			Err: err,
		}
	}()
}

// ClearTimeout is not supported by the ApplyTask.
func (a *ApplyTask) ClearTimeout() {}

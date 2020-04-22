// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ApplyTask applies the given Objects to the cluster
// by using the ApplyOptions.
type ApplyTask struct {
	ApplyOptions applyOptions
	Objects      []*resource.Info
}

// applyOptions defines the two key functions on the ApplyOptions
// struct that is used by the ApplyTask.
type applyOptions interface {

	// Run applies the resource set with the SetObjects function
	// to the cluster.
	Run() error

	// SetObjects sets the slice of resource (in the form form info objects)
	// that will be applied upon invoking the Run function.
	SetObjects([]*resource.Info)
}

// Start creates a new goroutine that will invoke
// the Run function on the ApplyOptions to update
// the cluster. It will push a TaskResult on the taskChannel
// to signal to the taskrunner that the task has completed (or failed).
// It will also fetch the Generation from each of the applied resources
// after the Run function has completed. This information is then added
// to the taskContext. The generation is increased every time
// the desired state of a resource is changed.
func (a *ApplyTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		a.ApplyOptions.SetObjects(a.Objects)
		err := a.ApplyOptions.Run()
		for _, info := range a.Objects {
			id := object.InfoToObjMeta(info)
			acc, _ := meta.Accessor(info.Object)
			gen := acc.GetGeneration()
			taskContext.ResourceApplied(id, gen)
		}
		taskContext.TaskChannel() <- taskrunner.TaskResult{
			Err: err,
		}
	}()
}

// ClearTimeout is not supported by the ApplyTask.
func (a *ApplyTask) ClearTimeout() {}

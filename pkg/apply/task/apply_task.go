// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ApplyTask applies the given Objects to the cluster
// by using the ApplyOptions.
type ApplyTask struct {
	ApplyOptions applyOptions
	InfoHelper   info.InfoHelper
	Objects      []*resource.Info
	DryRun       bool
}

// applyOptions defines the two key functions on the ApplyOptions
// struct that is used by the ApplyTask.
type applyOptions interface {

	// Run applies the resource set with the SetObjects function
	// to the cluster.
	Run() error

	// SetObjects sets the slice of resource (in the form form resourceInfo objects)
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
		// Update the dry-run field on the Applier.
		a.setDryRunField()
		// Set the client and mapping fields on the provided
		// infos so they can be applied to the cluster.
		err := a.InfoHelper.UpdateInfos(a.Objects)
		if err != nil {
			a.sendTaskResult(taskContext, err)
			return
		}
		a.ApplyOptions.SetObjects(a.Objects)
		err = a.ApplyOptions.Run()
		if err != nil {
			a.sendTaskResult(taskContext, err)
			return
		}
		// Fetch the Generation from all Infos after they have been
		// applied.
		for _, obj := range a.Objects {
			id := object.InfoToObjMeta(obj)
			acc, _ := meta.Accessor(obj.Object)
			gen := acc.GetGeneration()
			taskContext.ResourceApplied(id, gen)
		}
		err = a.InfoHelper.ResetRESTMapper()
		if err != nil {
			a.sendTaskResult(taskContext, err)
			return
		}
		a.sendTaskResult(taskContext, nil)
	}()
}

func (a *ApplyTask) sendTaskResult(taskContext *taskrunner.TaskContext, err error) {
	taskContext.TaskChannel() <- taskrunner.TaskResult{
		Err: err,
	}
}

func (a *ApplyTask) setDryRunField() {
	if ao, ok := a.ApplyOptions.(*apply.ApplyOptions); ok {
		ao.DryRun = a.DryRun
	}
}

// ClearTimeout is not supported by the ApplyTask.
func (a *ApplyTask) ClearTimeout() {}

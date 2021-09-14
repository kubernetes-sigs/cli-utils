// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// PruneTask prunes objects from the cluster
// by using the PruneOptions. The provided Objects is the
// set of resources that have just been applied.
type PruneTask struct {
	TaskName string

	Options      prune.Options
	PruneOptions *prune.Pruner
	Objects      []*unstructured.Unstructured
	Filters      []filter.ValidationFilter
}

func (p *PruneTask) Name() string {
	return p.TaskName
}

func (p *PruneTask) Action() event.ResourceAction {
	action := event.PruneAction
	if p.Options.Destroy {
		action = event.DeleteAction
	}
	return action
}

func (p *PruneTask) Identifiers() []object.ObjMetadata {
	return object.UnstructuredsToObjMetasOrDie(p.Objects)
}

// Start creates a new goroutine that will invoke
// the Run function on the PruneOptions to update
// the cluster. It will push a TaskResult on the taskChannel
// to signal to the taskrunner that the task has completed (or failed).
func (p *PruneTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		klog.V(2).Infof("prune task starting (%d objects)", len(p.Objects))
		// Create filter to prevent deletion of currently applied
		// objects. Must be done here to wait for applied UIDs.
		uidFilter := filter.CurrentUIDFilter{
			CurrentUIDs: taskContext.AppliedResourceUIDs(),
		}
		p.Filters = append(p.Filters, uidFilter)
		err := p.PruneOptions.Prune(
			taskContext,
			p.Objects,
			p.Filters,
			p.Options,
		)
		taskContext.TaskChannel() <- taskrunner.TaskResult{Err: err}
	}()
}

// OnStatusEvent is not supported by PruneTask.
func (p *PruneTask) OnStatusEvent(taskContext *taskrunner.TaskContext, e event.StatusEvent) {}

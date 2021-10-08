// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// PruneTask prunes objects from the cluster
// by using the PruneOptions. The provided Objects is the
// set of resources that have just been applied.
type PruneTask struct {
	TaskName string

	PruneOptions      *prune.PruneOptions
	Objects           object.UnstructuredSet
	Filters           []filter.ValidationFilter
	DryRunStrategy    common.DryRunStrategy
	PropagationPolicy metav1.DeletionPropagation
	// True if we are destroying, which deletes the inventory object
	// as well (possibly) the inventory namespace.
	Destroy bool
}

func (p *PruneTask) Name() string {
	return p.TaskName
}

func (p *PruneTask) Action() event.ResourceAction {
	action := event.PruneAction
	if p.Destroy {
		action = event.DeleteAction
	}
	return action
}

func (p *PruneTask) Identifiers() object.ObjMetadataSet {
	return object.UnstructuredsToObjMetasOrDie(p.Objects)
}

// Start creates a new goroutine that will invoke
// the Run function on the PruneOptions to update
// the cluster. It will push a TaskResult on the taskChannel
// to signal to the taskrunner that the task has completed (or failed).
func (p *PruneTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		klog.V(2).Infof("prune task starting (objects: %d, name: %q)", len(p.Objects), p.Name())
		// Create filter to prevent deletion of currently applied
		// objects. Must be done here to wait for applied UIDs.
		uidFilter := filter.CurrentUIDFilter{
			CurrentUIDs: taskContext.AppliedResourceUIDs(),
		}
		p.Filters = append(p.Filters, uidFilter)
		err := p.PruneOptions.Prune(
			p.Objects,
			p.Filters,
			taskContext,
			p.Name(),
			prune.Options{
				DryRunStrategy:    p.DryRunStrategy,
				PropagationPolicy: p.PropagationPolicy,
				Destroy:           p.Destroy,
			},
		)
		taskContext.TaskChannel() <- taskrunner.TaskResult{
			Err: err,
		}
	}()
}

// ClearTimeout is not supported by the PruneTask.
func (p *PruneTask) ClearTimeout() {}

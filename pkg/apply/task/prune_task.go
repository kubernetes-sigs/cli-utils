// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// PruneTask prunes objects from the cluster
// by using the PruneOptions. The provided Objects is the
// set of resources that have just been applied.
type PruneTask struct {
	TaskName string

	PruneOptions      *prune.PruneOptions
	InventoryObject   inventory.InventoryInfo
	Objects           []*unstructured.Unstructured
	DryRunStrategy    common.DryRunStrategy
	PropagationPolicy metav1.DeletionPropagation
	InventoryPolicy   inventory.InventoryPolicy
}

func (p *PruneTask) Name() string {
	return p.TaskName
}

func (p *PruneTask) Action() event.ResourceAction {
	action := event.PruneAction
	if p.PruneOptions.Destroy {
		action = event.DeleteAction
	}
	return action
}

func (p *PruneTask) Identifiers() []object.ObjMetadata {
	return object.UnstructuredsToObjMetas(p.Objects)
}

// Start creates a new goroutine that will invoke
// the Run function on the PruneOptions to update
// the cluster. It will push a TaskResult on the taskChannel
// to signal to the taskrunner that the task has completed (or failed).
func (p *PruneTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		currentUIDs := taskContext.AllResourceUIDs()
		err := p.PruneOptions.Prune(p.InventoryObject, p.Objects,
			currentUIDs, taskContext, prune.Options{
				DryRunStrategy:    p.DryRunStrategy,
				PropagationPolicy: p.PropagationPolicy,
				InventoryPolicy:   p.InventoryPolicy,
			})
		taskContext.TaskChannel() <- taskrunner.TaskResult{
			Err: err,
		}
	}()
}

// ClearTimeout is not supported by the PruneTask.
func (p *PruneTask) ClearTimeout() {}

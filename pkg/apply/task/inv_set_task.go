// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/klog"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// InvSetTask encapsulates structures necessary to set the
// inventory references at the end of the apply/prune.
type InvSetTask struct {
	InvClient inventory.InventoryClient
	InvInfo   inventory.InventoryInfo
}

func (i *InvSetTask) Name() string {
	return "inventory-replace"
}

func (i *InvSetTask) Action() event.ResourceAction {
	return event.InventoryAction
}

func (i *InvSetTask) Identifiers() []object.ObjMetadata {
	return []object.ObjMetadata{}
}

// Start sets the inventory using the resources applied and the
// prunes that failed. This task must run after all the apply
// and prune tasks have completed.
func (i *InvSetTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		appliedObjs := taskContext.AppliedResources()
		klog.V(4).Infof("set inventory %d applied objects", len(appliedObjs))
		pruneFailures := taskContext.PruneFailures()
		klog.V(4).Infof("set inventory %d prune failures", len(pruneFailures))
		invObjs := object.Union(appliedObjs, pruneFailures)
		klog.V(4).Infof("set inventory %d total objects", len(invObjs))
		err := i.InvClient.Replace(i.InvInfo, invObjs)
		taskContext.TaskChannel() <- taskrunner.TaskResult{Err: err}
	}()
}

// ClearTimeout is not supported by the InvSetTask.
func (i *InvSetTask) ClearTimeout() {}

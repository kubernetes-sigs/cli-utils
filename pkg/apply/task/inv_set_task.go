// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// InvSetTask encapsulates structures necessary to set the
// inventory references at the end of the apply/prune.
type InvSetTask struct {
	TaskName      string
	InvClient     inventory.InventoryClient
	InvInfo       inventory.InventoryInfo
	PrevInventory map[object.ObjMetadata]bool
	DryRun        common.DryRunStrategy
}

func (i *InvSetTask) Name() string {
	return i.TaskName
}

func (i *InvSetTask) Action() event.ResourceAction {
	return event.InventoryAction
}

func (i *InvSetTask) Identifiers() object.ObjMetadataSet {
	return object.ObjMetadataSet{}
}

// Start sets the inventory using the resources applied and the
// prunes that failed. This task must run after all the apply
// and prune tasks have completed.
func (i *InvSetTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		klog.V(2).Infoln("starting inventory replace task")
		appliedObjs := taskContext.AppliedResources()
		klog.V(4).Infof("set inventory %d applied objects", len(appliedObjs))
		// If an object failed to apply, but it was previously stored in
		// the inventory, then keep it in the inventory so we don't lose
		// track of it for next apply/prune. An object not found in the cluster
		// is NOT stored as an apply failure (so it is properly removed from the inventory).
		applyFailures := object.ObjMetadataSet{}
		for _, failure := range taskContext.ResourceFailures() {
			if _, exists := i.PrevInventory[failure]; exists {
				applyFailures = append(applyFailures, failure)
			}
		}
		klog.V(4).Infof("keep in inventory %d applied failures", len(applyFailures))
		pruneFailures := taskContext.PruneFailures()
		klog.V(4).Infof("set inventory %d prune failures", len(pruneFailures))
		allApplyObjs := appliedObjs.Union(applyFailures)
		invObjs := allApplyObjs.Union(pruneFailures)
		klog.V(4).Infof("set inventory %d total objects", len(invObjs))
		err := i.InvClient.Replace(i.InvInfo, invObjs, i.DryRun)
		taskContext.TaskChannel() <- taskrunner.TaskResult{Err: err}
	}()
}

// ClearTimeout is not supported by the InvSetTask.
func (i *InvSetTask) ClearTimeout() {}

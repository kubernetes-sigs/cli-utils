// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// DeleteInvTask encapsulates structures necessary to delete
// the inventory object from the cluster. Implements
// the Task interface. This task should happen after all
// resources have been deleted.
type DeleteInvTask struct {
	TaskName  string
	InvClient inventory.InventoryClient
	InvInfo   inventory.InventoryInfo
	DryRun    common.DryRunStrategy
}

func (i *DeleteInvTask) Name() string {
	return i.TaskName
}

func (i *DeleteInvTask) Action() event.ResourceAction {
	return event.InventoryAction
}

func (i *DeleteInvTask) Identifiers() object.ObjMetadataSet {
	return object.ObjMetadataSet{}
}

// Start deletes the inventory object from the cluster.
func (i *DeleteInvTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		klog.V(2).Infoln("starting delete inventory task")
		err := i.InvClient.DeleteInventoryObj(i.InvInfo, i.DryRun)
		// Not found is not error, since this means it was already deleted.
		if apierrors.IsNotFound(err) {
			err = nil
		}
		taskContext.TaskChannel() <- taskrunner.TaskResult{Err: err}
	}()
}

// ClearTimeout is not supported by the DeleteInvTask.
func (i *DeleteInvTask) ClearTimeout() {}

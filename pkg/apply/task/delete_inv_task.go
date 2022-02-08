// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"

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
	InvClient inventory.Client
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
		klog.V(2).Infof("delete inventory task starting (name: %q)", i.Name())

		var invObjIds object.ObjMetadataSet

		im := taskContext.InventoryManager()

		failedDeletes := im.FailedDeletes()
		if len(failedDeletes) > 0 {
			invObjIds = append(invObjIds, failedDeletes...)
		}

		skippedDeletes := im.SkippedDeletes()
		if len(skippedDeletes) > 0 {
			invObjIds = append(invObjIds, skippedDeletes...)
		}

		failedRecs := im.FailedReconciles()
		if len(failedRecs) > 0 {
			invObjIds = append(invObjIds, failedRecs...)
		}

		skippedRecs := im.SkippedReconciles()
		if len(skippedRecs) > 0 {
			invObjIds = append(invObjIds, skippedRecs...)
		}

		timeoutRecs := im.TimeoutReconciles()
		if len(timeoutRecs) > 0 {
			invObjIds = append(invObjIds, timeoutRecs...)
		}

		if len(invObjIds) > 0 {
			invObjIds = invObjIds.Unique()

			klog.V(4).Infof("set inventory %d total objects", len(invObjIds))
			inv := im.Inventory()
			// TODO: move these inventory updates to the other tasks
			inv.Spec.Objects = inventory.ObjectReferencesFromObjMetadataSet(invObjIds)
			// TODO: update inventory status?
			err := i.InvClient.Store(inv)
			if err != nil {
				err = fmt.Errorf("failed to update inventory: %w", err)
				i.sendTaskResult(taskContext, err)
				return
			}

			i.sendTaskResult(taskContext, nil)
			return
		}

		invInfo := inventory.InventoryInfoFromObject(im.Inventory())
		err := i.InvClient.Delete(invInfo)
		if err != nil {
			err = fmt.Errorf("failed to delete inventory: %w", err)
			i.sendTaskResult(taskContext, err)
			return
		}

		i.sendTaskResult(taskContext, nil)
	}()
}

// Cancel is not supported by the DeleteInvTask.
func (i *DeleteInvTask) Cancel(_ *taskrunner.TaskContext) {}

// StatusUpdate is not supported by the DeleteInvTask.
func (i *DeleteInvTask) StatusUpdate(_ *taskrunner.TaskContext, _ object.ObjMetadata) {}

func (i *DeleteInvTask) sendTaskResult(taskContext *taskrunner.TaskContext, err error) {
	klog.V(2).Infof("delete inventory task completing (name: %q)", i.Name())
	taskContext.TaskChannel() <- taskrunner.TaskResult{
		Err: err,
	}
}

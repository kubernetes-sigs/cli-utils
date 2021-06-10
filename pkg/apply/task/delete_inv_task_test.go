// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestDeleteInvTask(t *testing.T) {
	client := inventory.NewFakeInventoryClient([]object.ObjMetadata{})
	eventChannel := make(chan event.Event)
	context := taskrunner.NewTaskContext(eventChannel)
	task := DeleteInvTask{
		TaskName:  taskName,
		InvClient: client,
		InvInfo:   localInv,
	}
	if taskName != task.Name() {
		t.Errorf("expected task name (%s), got (%s)", taskName, task.Name())
	}
	task.Start(context)
	result := <-context.TaskChannel()
	if result.Err != nil {
		t.Errorf("unexpected error running DeleteInvTask: %s", result.Err)
	}
}

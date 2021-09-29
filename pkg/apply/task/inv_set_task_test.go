// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestInvSetTask(t *testing.T) {
	id1 := object.UnstructuredToObjMetaOrDie(obj1)
	id2 := object.UnstructuredToObjMetaOrDie(obj2)
	id3 := object.UnstructuredToObjMetaOrDie(obj3)

	tests := map[string]struct {
		appliedObjs   []object.ObjMetadata
		applyFailures []object.ObjMetadata
		prevInventory []object.ObjMetadata
		pruneFailures []object.ObjMetadata
		expectedObjs  []object.ObjMetadata
	}{
		"no apply objs, no prune failures; no inventory": {
			appliedObjs:   []object.ObjMetadata{},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{},
		},
		"one apply objs, no prune failures; one inventory": {
			appliedObjs:   []object.ObjMetadata{id1},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{id1},
		},
		"no apply objs, one prune failures; one inventory": {
			appliedObjs:   []object.ObjMetadata{},
			pruneFailures: []object.ObjMetadata{id1},
			expectedObjs:  []object.ObjMetadata{id1},
		},
		"one apply objs, one prune failures; one inventory": {
			appliedObjs:   []object.ObjMetadata{id3},
			pruneFailures: []object.ObjMetadata{id3},
			expectedObjs:  []object.ObjMetadata{id3},
		},
		"two apply objs, two prune failures; three inventory": {
			appliedObjs:   []object.ObjMetadata{id1, id2},
			pruneFailures: []object.ObjMetadata{id2, id3},
			expectedObjs:  []object.ObjMetadata{id1, id2, id3},
		},
		"no apply objs, no apply failures, no prune failures; no inventory": {
			appliedObjs:   []object.ObjMetadata{},
			applyFailures: []object.ObjMetadata{id3},
			prevInventory: []object.ObjMetadata{},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{},
		},
		"one apply failure not in prev inventory; no inventory": {
			appliedObjs:   []object.ObjMetadata{},
			applyFailures: []object.ObjMetadata{id3},
			prevInventory: []object.ObjMetadata{},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{},
		},
		"one apply obj, one apply failure not in prev inventory; one inventory": {
			appliedObjs:   []object.ObjMetadata{id2},
			applyFailures: []object.ObjMetadata{id3},
			prevInventory: []object.ObjMetadata{},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{id2},
		},
		"one apply obj, one apply failure in prev inventory; one inventory": {
			appliedObjs:   []object.ObjMetadata{id2},
			applyFailures: []object.ObjMetadata{id3},
			prevInventory: []object.ObjMetadata{id3},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{id2, id3},
		},
		"one apply obj, two apply failures with one in prev inventory; two inventory": {
			appliedObjs:   []object.ObjMetadata{id2},
			applyFailures: []object.ObjMetadata{id1, id3},
			prevInventory: []object.ObjMetadata{id3},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{id2, id3},
		},
		"three apply failures with two in prev inventory; two inventory": {
			appliedObjs:   []object.ObjMetadata{},
			applyFailures: []object.ObjMetadata{id1, id2, id3},
			prevInventory: []object.ObjMetadata{id2, id3},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{id2, id3},
		},
		"three apply failures with three in prev inventory; three inventory": {
			appliedObjs:   []object.ObjMetadata{},
			applyFailures: []object.ObjMetadata{id1, id2, id3},
			prevInventory: []object.ObjMetadata{id2, id3, id1},
			pruneFailures: []object.ObjMetadata{},
			expectedObjs:  []object.ObjMetadata{id2, id1, id3},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := inventory.NewFakeInventoryClient([]object.ObjMetadata{})
			eventChannel := make(chan event.Event)
			resourceCache := cache.NewResourceCacheMap()
			context := taskrunner.NewTaskContext(eventChannel, resourceCache)

			prevInventory := make(map[object.ObjMetadata]bool, len(tc.prevInventory))
			for _, prevInvID := range tc.prevInventory {
				prevInventory[prevInvID] = true
			}
			task := InvSetTask{
				TaskName:      taskName,
				InvClient:     client,
				InvInfo:       nil,
				PrevInventory: prevInventory,
			}
			for _, applyObj := range tc.appliedObjs {
				context.ResourceApplied(applyObj, "unusued-uid", int64(0))
			}
			for _, applyFailure := range tc.applyFailures {
				context.CaptureResourceFailure(applyFailure)
			}
			for _, pruneObj := range tc.pruneFailures {
				context.CapturePruneFailure(pruneObj)
			}
			if taskName != task.Name() {
				t.Errorf("expected task name (%s), got (%s)", taskName, task.Name())
			}
			task.Start(context)
			result := <-context.TaskChannel()
			if result.Err != nil {
				t.Errorf("unexpected error running InvAddTask: %s", result.Err)
			}
			actual, _ := client.GetClusterObjs(nil, common.DryRunNone)
			if !object.SetEquals(tc.expectedObjs, actual) {
				t.Errorf("expected merged inventory (%s), got (%s)", tc.expectedObjs, actual)
			}
		})
	}
}

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
		appliedObjs   object.ObjMetadataSet
		applyFailures object.ObjMetadataSet
		prevInventory object.ObjMetadataSet
		pruneFailures object.ObjMetadataSet
		expectedObjs  object.ObjMetadataSet
	}{
		"no apply objs, no prune failures; no inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"one apply objs, no prune failures; one inventory": {
			appliedObjs:   object.ObjMetadataSet{id1},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id1},
		},
		"no apply objs, one prune failures; one inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			pruneFailures: object.ObjMetadataSet{id1},
			expectedObjs:  object.ObjMetadataSet{id1},
		},
		"one apply objs, one prune failures; one inventory": {
			appliedObjs:   object.ObjMetadataSet{id3},
			pruneFailures: object.ObjMetadataSet{id3},
			expectedObjs:  object.ObjMetadataSet{id3},
		},
		"two apply objs, two prune failures; three inventory": {
			appliedObjs:   object.ObjMetadataSet{id1, id2},
			pruneFailures: object.ObjMetadataSet{id2, id3},
			expectedObjs:  object.ObjMetadataSet{id1, id2, id3},
		},
		"no apply objs, no apply failures, no prune failures; no inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			applyFailures: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"one apply failure not in prev inventory; no inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			applyFailures: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{},
		},
		"one apply obj, one apply failure not in prev inventory; one inventory": {
			appliedObjs:   object.ObjMetadataSet{id2},
			applyFailures: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2},
		},
		"one apply obj, one apply failure in prev inventory; one inventory": {
			appliedObjs:   object.ObjMetadataSet{id2},
			applyFailures: object.ObjMetadataSet{id3},
			prevInventory: object.ObjMetadataSet{id3},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id3},
		},
		"one apply obj, two apply failures with one in prev inventory; two inventory": {
			appliedObjs:   object.ObjMetadataSet{id2},
			applyFailures: object.ObjMetadataSet{id1, id3},
			prevInventory: object.ObjMetadataSet{id3},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id3},
		},
		"three apply failures with two in prev inventory; two inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			applyFailures: object.ObjMetadataSet{id1, id2, id3},
			prevInventory: object.ObjMetadataSet{id2, id3},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id3},
		},
		"three apply failures with three in prev inventory; three inventory": {
			appliedObjs:   object.ObjMetadataSet{},
			applyFailures: object.ObjMetadataSet{id1, id2, id3},
			prevInventory: object.ObjMetadataSet{id2, id3, id1},
			pruneFailures: object.ObjMetadataSet{},
			expectedObjs:  object.ObjMetadataSet{id2, id1, id3},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := inventory.NewFakeInventoryClient(object.ObjMetadataSet{})
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
			if !tc.expectedObjs.Equal(actual) {
				t.Errorf("expected merged inventory (%s), got (%s)", tc.expectedObjs, actual)
			}
		})
	}
}

// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var pod1Inv = &object.ObjMetadata{
	Namespace: testNamespace,
	Name:      pod1Name,
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Pod",
	},
}

var pod2Inv = &object.ObjMetadata{
	Namespace: testNamespace,
	Name:      pod2Name,
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Pod",
	},
}

var pod3Inv = &object.ObjMetadata{
	Namespace: testNamespace,
	Name:      pod3Name,
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "Pod",
	},
}

var groupingInv = &object.ObjMetadata{
	Namespace: testNamespace,
	Name:      groupingObjName,
	GroupKind: schema.GroupKind{
		Group: "",
		Kind:  "ConfigMap",
	},
}

func TestInfoToObjMetadata(t *testing.T) {
	tests := map[string]struct {
		info     *resource.Info
		expected *object.ObjMetadata
		isError  bool
	}{
		"Nil info is an error": {
			info:     nil,
			expected: nil,
			isError:  true,
		},
		"Nil info object is an error": {
			info:     nilInfo,
			expected: nil,
			isError:  true,
		},
		"Pod 1 object becomes Pod 1 object metadata": {
			info:     pod1Info,
			expected: pod1Inv,
			isError:  false,
		},
		"Grouping object becomes grouping object metadata": {
			info:     copyGroupingInfo(),
			expected: groupingInv,
			isError:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := infoToObjMetadata(tc.info)
			if tc.isError && err == nil {
				t.Errorf("Did not receive expected error.\n")
			}
			if !tc.isError {
				if err != nil {
					t.Errorf("Receieved unexpected error: %s\n", err)
				}
				if !tc.expected.Equals(actual) {
					t.Errorf("Expected ObjMetadata (%s), got (%s)\n", tc.expected, actual)
				}
			}
		})
	}
}

// Returns a grouping object with the inventory set from
// the passed "children".
func createGroupingInfo(name string, children ...*resource.Info) *resource.Info {
	groupingName := groupingObjName
	if len(name) > 0 {
		groupingName = name
	}
	groupingObjCopy := groupingObj.DeepCopy()
	var groupingInfo = &resource.Info{
		Namespace: testNamespace,
		Name:      groupingName,
		Object:    groupingObjCopy,
	}
	infos := []*resource.Info{groupingInfo}
	infos = append(infos, children...)
	_ = AddInventoryToGroupingObj(infos)
	return groupingInfo
}

func TestUnionPastInventory(t *testing.T) {
	tests := map[string]struct {
		groupingInfos []*resource.Info
		expected      []*object.ObjMetadata
	}{
		"Empty grouping objects = empty inventory": {
			groupingInfos: []*resource.Info{},
			expected:      []*object.ObjMetadata{},
		},
		"No children in grouping object, equals no inventory": {
			groupingInfos: []*resource.Info{createGroupingInfo("test-1")},
			expected:      []*object.ObjMetadata{},
		},
		"Grouping object with Pod1 returns inventory with Pod1": {
			groupingInfos: []*resource.Info{createGroupingInfo("test-1", pod1Info)},
			expected:      []*object.ObjMetadata{pod1Inv},
		},
		"Grouping object with three pods returns inventory with three pods": {
			groupingInfos: []*resource.Info{
				createGroupingInfo("test-1", pod1Info, pod2Info, pod3Info),
			},
			expected: []*object.ObjMetadata{pod1Inv, pod2Inv, pod3Inv},
		},
		"Two grouping objects with different pods returns inventory with both pods": {
			groupingInfos: []*resource.Info{
				createGroupingInfo("test-1", pod1Info),
				createGroupingInfo("test-2", pod2Info),
			},
			expected: []*object.ObjMetadata{pod1Inv, pod2Inv},
		},
		"Two grouping objects with overlapping pods returns set of pods": {
			groupingInfos: []*resource.Info{
				createGroupingInfo("test-1", pod1Info, pod2Info),
				createGroupingInfo("test-2", pod2Info, pod3Info),
			},
			expected: []*object.ObjMetadata{pod1Inv, pod2Inv, pod3Inv},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := unionPastInventory(tc.groupingInfos)
			expected := NewInventory(tc.expected)
			if err != nil {
				t.Errorf("Unexpected error received: %s\n", err)
			}
			if !expected.Equals(actual) {
				t.Errorf("Expected inventory (%s), got (%s)\n", expected, actual)
			}
		})
	}
}

func TestPrune(t *testing.T) {
	tests := map[string]struct {
		// pastInfos/currentInfos do NOT contain the grouping object.
		// Grouping object is generated from these past/current objects.
		pastInfos    []*resource.Info
		currentInfos []*resource.Info
		prunedInfos  []*resource.Info
		isError      bool
	}{
		"Past and current objects are empty; no pruned objects": {
			pastInfos:    []*resource.Info{},
			currentInfos: []*resource.Info{},
			prunedInfos:  []*resource.Info{},
			isError:      false,
		},
		"Past and current objects are the same; no pruned objects": {
			pastInfos:    []*resource.Info{pod1Info, pod2Info},
			currentInfos: []*resource.Info{pod2Info, pod1Info},
			prunedInfos:  []*resource.Info{},
			isError:      false,
		},
		"No past objects; no pruned objects": {
			pastInfos:    []*resource.Info{},
			currentInfos: []*resource.Info{pod2Info, pod1Info},
			prunedInfos:  []*resource.Info{},
			isError:      false,
		},
		"No current objects; all previous objects pruned": {
			pastInfos:    []*resource.Info{pod1Info, pod2Info, pod3Info},
			currentInfos: []*resource.Info{},
			prunedInfos:  []*resource.Info{pod1Info, pod2Info, pod3Info},
			isError:      false,
		},
		"Omitted object is pruned": {
			pastInfos:    []*resource.Info{pod1Info, pod2Info},
			currentInfos: []*resource.Info{pod2Info, pod3Info},
			prunedInfos:  []*resource.Info{pod1Info},
			isError:      false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			po := NewPruneOptions(populateObjectIds(tc.currentInfos, t))
			po.DryRun = true
			// Set up the previously applied objects.
			pastGroupingInfo := createGroupingInfo("past-group", tc.pastInfos...)
			po.pastGroupingObjects = []*resource.Info{pastGroupingInfo}
			po.retrievedGroupingObjects = true
			// Set up the currently applied objects.
			currentGroupingInfo := createGroupingInfo("current-group", tc.currentInfos...)
			currentInfos := append(tc.currentInfos, currentGroupingInfo)
			// The event channel can not block; make sure its bigger than all
			// the events that can be put on it.
			eventChannel := make(chan event.Event, len(tc.pastInfos)+1) // Add one for grouping object
			defer close(eventChannel)
			// Set up the fake dynamic client to recognize all objects, and the RESTMapper.
			po.client = fake.NewSimpleDynamicClient(scheme.Scheme,
				pod1Info.Object, pod2Info.Object, pod3Info.Object)
			po.mapper = testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
				scheme.Scheme.PrioritizedVersionsAllGroups()...)
			// Run the prune and validate.
			err := po.Prune(currentInfos, eventChannel)
			if !tc.isError {
				if err != nil {
					t.Fatalf("Unexpected error during Prune(): %#v", err)
				}
				// Validate the prune events on the event channel.
				expectedPruneEvents := len(tc.prunedInfos) + 1 // One extra for pruning grouping object
				actualPruneEvents := len(eventChannel)
				if expectedPruneEvents != actualPruneEvents {
					t.Errorf("Expected (%d) prune events, got (%d)",
						expectedPruneEvents, actualPruneEvents)
				}
			} else if err == nil {
				t.Fatalf("Expected error during Prune() but received none")
			}
		})
	}
}

// populateObjectIds returns a pointer to a set of strings containing
// the UID's of the passed objects (infos).
func populateObjectIds(infos []*resource.Info, t *testing.T) sets.String {
	uids := sets.NewString()
	for _, currInfo := range infos {
		currObj := currInfo.Object
		metadata, err := meta.Accessor(currObj)
		if err != nil {
			t.Fatalf("Unexpected error retrieving object metadata: %#v", err)
		}
		uid := string(metadata.GetUID())
		uids.Insert(uid)
	}
	return uids
}

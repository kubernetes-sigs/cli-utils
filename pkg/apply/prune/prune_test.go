// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
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
func createGroupingInfo(_ string, children ...*resource.Info) *resource.Info {
	groupingObjCopy := groupingObj.DeepCopy()
	var groupingInfo = &resource.Info{
		Namespace: testNamespace,
		Name:      groupingObjName,
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

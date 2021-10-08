// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package object

import (
	"testing"

	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
)

var resources = map[string]string{
	"pod1": `
apiVersion: v1
kind: Pod
metadata:
  name: pod1
  namespace: default
`,
	"pod1dupe": `
apiVersion: v1
kind: Pod
metadata:
  name: pod1
  namespace: default
`,
	"pod2": `
apiVersion: v1
kind: Pod
metadata:
  name: pod2
  namespace: default
`,
	"pod3": `
apiVersion: v1
kind: Pod
metadata:
  name: pod3
  namespace: default
`,
	"pod4": `
apiVersion: v1
kind: Pod
metadata:
name: pod4
namespace: default
`,
}

func TestUnstructuredSetEquals(t *testing.T) {
	pod1 := testutil.YamlToUnstructured(t, resources["pod1"])
	pod1dupe := testutil.YamlToUnstructured(t, resources["pod1dupe"])
	pod2 := testutil.YamlToUnstructured(t, resources["pod2"])
	pod3 := testutil.YamlToUnstructured(t, resources["pod3"])
	pod4 := testutil.YamlToUnstructured(t, resources["pod4"])

	testCases := map[string]struct {
		setA    UnstructuredSet
		setB    UnstructuredSet
		isEqual bool
	}{
		"Empty sets": {
			setA:    UnstructuredSet{},
			setB:    UnstructuredSet{},
			isEqual: true,
		},
		"Empty first set": {
			setA:    UnstructuredSet{},
			setB:    UnstructuredSet{pod1, pod2},
			isEqual: false,
		},
		"Empty second set": {
			setA:    UnstructuredSet{pod1, pod2},
			setB:    UnstructuredSet{},
			isEqual: false,
		},
		"Different order": {
			setA:    UnstructuredSet{pod1, pod2},
			setB:    UnstructuredSet{pod2, pod1},
			isEqual: true,
		},
		"One item overlap": {
			setA:    UnstructuredSet{pod1, pod2},
			setB:    UnstructuredSet{pod2, pod3},
			isEqual: false,
		},
		"Disjoint sets": {
			setA:    UnstructuredSet{pod1, pod2},
			setB:    UnstructuredSet{pod3, pod4},
			isEqual: false,
		},
		"Duplicate pointer": {
			setA:    UnstructuredSet{pod1, pod1},
			setB:    UnstructuredSet{pod1},
			isEqual: true,
		},
		"Duplicate value": {
			setA:    UnstructuredSet{pod1, pod1dupe},
			setB:    UnstructuredSet{pod1},
			isEqual: true,
		},
		"Same value": {
			setA:    UnstructuredSet{pod1},
			setB:    UnstructuredSet{pod1dupe},
			isEqual: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := tc.setA.Equal(tc.setB)
			if tc.isEqual != actual {
				t.Errorf("Equal expected (%t), got (%t)", tc.isEqual, actual)
			}
		})
	}
}

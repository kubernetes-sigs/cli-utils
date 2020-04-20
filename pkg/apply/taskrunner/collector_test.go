// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestCollector_ConditionMet(t *testing.T) {
	identifiers := map[string]object.ObjMetadata{
		"dep": {
			GroupKind: schema.GroupKind{
				Group: "apps",
				Kind:  "Deployment",
			},
			Name:      "Foo",
			Namespace: "default",
		},
		"custom": {
			GroupKind: schema.GroupKind{
				Group: "custom.io",
				Kind:  "Custom",
			},
			Name: "Foo",
		},
	}

	testCases := map[string]struct {
		collectorState map[object.ObjMetadata]resourceStatus
		waitTaskData   []resourceWaitData
		condition      condition
		expectedResult bool
	}{
		"single resource with current status": {
			collectorState: map[object.ObjMetadata]resourceStatus{
				identifiers["dep"]: {
					Identifier:    identifiers["dep"],
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(42),
				},
			},
			waitTaskData: []resourceWaitData{
				{
					identifier: identifiers["dep"],
					generation: int64(42),
				},
			},
			condition:      AllCurrent,
			expectedResult: true,
		},
		"single resource with current status and old generation": {
			collectorState: map[object.ObjMetadata]resourceStatus{
				identifiers["dep"]: {
					Identifier:    identifiers["dep"],
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(41),
				},
			},
			waitTaskData: []resourceWaitData{
				{
					identifier: identifiers["dep"],
					generation: int64(42),
				},
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
		"multiple resources not all current": {
			collectorState: map[object.ObjMetadata]resourceStatus{
				identifiers["dep"]: {
					Identifier:    identifiers["dep"],
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(41),
				},
				identifiers["custom"]: {
					Identifier:    identifiers["custom"],
					CurrentStatus: status.InProgressStatus,
					Generation:    int64(0),
				},
			},
			waitTaskData: []resourceWaitData{
				{
					identifier: identifiers["dep"],
					generation: int64(42),
				},
				{
					identifier: identifiers["custom"],
					generation: int64(0),
				},
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
		"multiple resources single with old generation": {
			collectorState: map[object.ObjMetadata]resourceStatus{
				identifiers["dep"]: {
					Identifier:    identifiers["dep"],
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(42),
				},
				identifiers["custom"]: {
					Identifier:    identifiers["custom"],
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(4),
				},
			},
			waitTaskData: []resourceWaitData{
				{
					identifier: identifiers["dep"],
					generation: int64(42),
				},
				{
					identifier: identifiers["custom"],
					generation: int64(5),
				},
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			rsc := newResourceStatusCollector([]object.ObjMetadata{})
			rsc.resourceMap = tc.collectorState

			res := rsc.conditionMet(tc.waitTaskData, tc.condition)

			assert.Equal(t, tc.expectedResult, res)
		})
	}
}

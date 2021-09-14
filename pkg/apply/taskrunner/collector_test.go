// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		collectorState map[object.ObjMetadata]ResourceStatus
		waitTaskData   []ResourceGeneration
		condition      Condition
		expectedResult bool
	}{
		"single resource with current status": {
			collectorState: map[object.ObjMetadata]ResourceStatus{
				identifiers["dep"]: {
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(42),
				},
			},
			waitTaskData: []ResourceGeneration{
				{
					Identifier: identifiers["dep"],
					Generation: int64(42),
				},
			},
			condition:      AllCurrent,
			expectedResult: true,
		},
		"single resource with current status and old generation": {
			collectorState: map[object.ObjMetadata]ResourceStatus{
				identifiers["dep"]: {
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(41),
				},
			},
			waitTaskData: []ResourceGeneration{
				{
					Identifier: identifiers["dep"],
					Generation: int64(42),
				},
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
		"multiple resources not all current": {
			collectorState: map[object.ObjMetadata]ResourceStatus{
				identifiers["dep"]: {
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(41),
				},
				identifiers["custom"]: {
					CurrentStatus: status.InProgressStatus,
					Generation:    int64(0),
				},
			},
			waitTaskData: []ResourceGeneration{
				{
					Identifier: identifiers["dep"],
					Generation: int64(42),
				},
				{
					Identifier: identifiers["custom"],
					Generation: int64(0),
				},
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
		"multiple resources single with old generation": {
			collectorState: map[object.ObjMetadata]ResourceStatus{
				identifiers["dep"]: {
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(42),
				},
				identifiers["custom"]: {
					CurrentStatus: status.CurrentStatus,
					Generation:    int64(4),
				},
			},
			waitTaskData: []ResourceGeneration{
				{
					Identifier: identifiers["dep"],
					Generation: int64(42),
				},
				{
					Identifier: identifiers["custom"],
					Generation: int64(5),
				},
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			rsc := NewResourceStatusCollector()
			rsc.resourceMap = tc.collectorState

			res := rsc.ConditionMet(tc.waitTaskData, tc.condition)

			assert.Equal(t, tc.expectedResult, res)
		})
	}
}

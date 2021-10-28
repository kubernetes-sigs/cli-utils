// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package taskrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	ktestutil "sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var deployment1y = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: Foo
  namespace: default
spec:
  replicas: 1
status:
  replicas: 1
  readyReplicas: 1
  updatedReplicas: 1
  availableReplicas: 1
  conditions:
  - status: "True"
    type: Available
  - status: "True"
    type: Ready
`

var custom1y = `
apiVersion: custom.io/v1alpha1
kind: Custom
metadata:
  name: Foo
  namespace: default
spec: {}
status:
conditions:
- status: "False"
  type: Ready
`

// withGeneration returns a DeepCopy with .metadata.generation set.
func withGeneration(obj *unstructured.Unstructured, gen int64) *unstructured.Unstructured {
	obj = obj.DeepCopy()
	obj.SetGeneration(gen)
	return obj
}

func TestCollector_ConditionMet(t *testing.T) {
	deployment1 := ktestutil.YamlToUnstructured(t, deployment1y)
	deployment1Meta := object.UnstructuredToObjMetaOrDie(deployment1)
	custom1 := ktestutil.YamlToUnstructured(t, custom1y)
	custom1Meta := object.UnstructuredToObjMetaOrDie(custom1)

	testCases := map[string]struct {
		cacheContents  []cache.ResourceStatus
		appliedGen     map[object.ObjMetadata]int64
		ids            object.ObjMetadataSet
		condition      Condition
		expectedResult bool
	}{
		"single resource with current status": {
			cacheContents: []cache.ResourceStatus{
				{
					Resource: withGeneration(deployment1, 42),
					Status:   status.CurrentStatus,
				},
			},
			appliedGen: map[object.ObjMetadata]int64{
				deployment1Meta: 42,
			},
			ids: object.ObjMetadataSet{
				deployment1Meta,
			},
			condition:      AllCurrent,
			expectedResult: true,
		},
		"single resource with current status and old generation": {
			cacheContents: []cache.ResourceStatus{
				{
					Resource: withGeneration(deployment1, 41),
					Status:   status.CurrentStatus,
				},
			},
			appliedGen: map[object.ObjMetadata]int64{
				deployment1Meta: 42,
			},
			ids: object.ObjMetadataSet{
				deployment1Meta,
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
		"multiple resources not all current": {
			cacheContents: []cache.ResourceStatus{
				{
					Resource: withGeneration(deployment1, 42),
					Status:   status.InProgressStatus,
				},
				{
					Resource: withGeneration(custom1, 0),
					Status:   status.CurrentStatus,
				},
			},
			appliedGen: map[object.ObjMetadata]int64{
				deployment1Meta: 42,
				custom1Meta:     0,
			},
			ids: object.ObjMetadataSet{
				deployment1Meta,
				custom1Meta,
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
		"multiple resources single with old generation": {
			cacheContents: []cache.ResourceStatus{
				{
					Resource: withGeneration(deployment1, 42),
					Status:   status.CurrentStatus,
				},
				{
					Resource: withGeneration(custom1, 4),
					Status:   status.CurrentStatus,
				},
			},
			appliedGen: map[object.ObjMetadata]int64{
				deployment1Meta: 42,
				custom1Meta:     5,
			},
			ids: object.ObjMetadataSet{
				deployment1Meta,
				custom1Meta,
			},
			condition:      AllCurrent,
			expectedResult: false,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			resourceCache := cache.NewResourceCacheMap()
			if tc.cacheContents != nil {
				err := resourceCache.Load(tc.cacheContents...)
				assert.NoError(t, err)
			}

			taskContext := NewTaskContext(nil, resourceCache)

			if tc.appliedGen != nil {
				for id, gen := range tc.appliedGen {
					taskContext.AddSuccessfulApply(id, types.UID("unused"), gen)
				}
			}

			res := conditionMet(taskContext, tc.ids, tc.condition)

			assert.Equal(t, tc.expectedResult, res)
		})
	}
}

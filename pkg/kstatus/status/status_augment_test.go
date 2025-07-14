// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var pod = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Running
`

var custom = `
apiVersion: v1beta1
kind: SomeCustomKind
metadata:
   generation: 1
   name: test
   namespace: default
`

var timestamp = time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)

func addConditions(t *testing.T, u *unstructured.Unstructured, conditions []map[string]any) {
	conds := make([]any, 0)
	for _, c := range conditions {
		conds = append(conds, c)
	}
	err := unstructured.SetNestedSlice(u.Object, conds, "status", "conditions")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAugmentConditions(t *testing.T) {
	testCases := map[string]struct {
		manifest           string
		withConditions     []map[string]any
		expectedConditions []Condition
	}{
		"no existing conditions": {
			manifest:       pod,
			withConditions: []map[string]any{},
			expectedConditions: []Condition{
				{
					Type:   ConditionReconciling,
					Status: corev1.ConditionTrue,
					Reason: "PodRunningNotReady",
				},
			},
		},
		"has other existing conditions": {
			manifest: pod,
			withConditions: []map[string]any{
				{
					"lastTransitionTime": timestamp,
					"lastUpdateTime":     timestamp,
					"type":               "Ready",
					"status":             "False",
					"reason":             "Pod has not started",
				},
			},
			expectedConditions: []Condition{
				{
					Type:   ConditionReconciling,
					Status: corev1.ConditionTrue,
					Reason: "PodRunningNotReady",
				},
				{
					Type:   "Ready",
					Status: corev1.ConditionFalse,
					Reason: "Pod has not started",
				},
			},
		},
		"already has condition of standard type InProgress": {
			manifest: pod,
			withConditions: []map[string]any{
				{
					"lastTransitionTime": timestamp,
					"lastUpdateTime":     timestamp,
					"type":               ConditionReconciling.String(),
					"status":             "True",
					"reason":             "PodIsAbsolutelyNotReady",
				},
			},
			expectedConditions: []Condition{
				{
					Type:   ConditionReconciling,
					Status: corev1.ConditionTrue,
					Reason: "PodIsAbsolutelyNotReady",
				},
			},
		},
		"already has condition of standard type Failed": {
			manifest: pod,
			withConditions: []map[string]any{
				{
					"lastTransitionTime": timestamp,
					"lastUpdateTime":     timestamp,
					"type":               ConditionStalled.String(),
					"status":             "True",
					"reason":             "PodHasFailed",
				},
			},
			expectedConditions: []Condition{
				{
					Type:   ConditionStalled,
					Status: corev1.ConditionTrue,
					Reason: "PodHasFailed",
				},
			},
		},
		"custom resource with no conditions": {
			manifest:           custom,
			withConditions:     []map[string]any{},
			expectedConditions: []Condition{},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			u := y2u(t, tc.manifest)
			addConditions(t, u, tc.withConditions)

			err := Augment(u)
			if err != nil {
				t.Error(err)
			}

			obj, err := GetObjectWithConditions(u.Object)
			if err != nil {
				t.Error(err)
			}

			assert.Equal(t, len(tc.expectedConditions), len(obj.Status.Conditions))

			for _, expectedCondition := range tc.expectedConditions {
				found := false
				for _, condition := range obj.Status.Conditions {
					if expectedCondition.Type.String() != condition.Type {
						continue
					}
					found = true
					assert.Equal(t, expectedCondition.Type.String(), condition.Type)
					assert.Equal(t, expectedCondition.Reason, condition.Reason)
				}
				assert.True(t, found)
			}
		})
	}
}

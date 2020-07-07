// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var pendingPod = `
apiVersion: v1
kind: Pod
metadata:
   generation: 1
   name: test
   namespace: qual
status:
   phase: Pending
`

var custom = `
apiVersion: v1beta1
kind: SomeCustomKind
metadata:
   generation: 1
   name: test
   namespace: default
`

var timestamp = time.Now().UTC().Truncate(time.Second)
var timestampStr = timestamp.Format(time.RFC3339)

func addConditions(t *testing.T, u *unstructured.Unstructured, conditions []map[string]interface{}) {
	conds := make([]interface{}, 0)
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
		manifest                string
		withConditions          []map[string]interface{}
		expectedConditions      []Condition
		checkLastTransitionTime bool
	}{
		"no existing conditions": {
			manifest:       pod,
			withConditions: []map[string]interface{}{},
			expectedConditions: []Condition{
				{
					Type:   ConditionReconciling,
					Status: corev1.ConditionTrue,
					Reason: "PodRunningNotReady",
				},
			},
		},
		"pendingPod with an extra condition without LastTransitionTime": {
			manifest: pendingPod,
			withConditions: []map[string]interface{}{
				{
					"type":   "PodScheduled",
					"status": "False",
					"reason": "Unknown",
				},
			},
			expectedConditions: []Condition{
				{
					Type:               ConditionReconciling,
					Status:             corev1.ConditionTrue,
					Reason:             "PodPending",
					LastTransitionTime: metav1.Time{},
				},
				{
					Type:               "PodScheduled",
					Status:             corev1.ConditionFalse,
					Reason:             "Unknown",
					LastTransitionTime: metav1.Time{},
				},
			},
			checkLastTransitionTime: true,
		},
		"pendingPod with an extra condition with LastTransitionTime": {
			manifest: pendingPod,
			withConditions: []map[string]interface{}{
				{
					"lastTransitionTime": timestampStr,
					"type":               "PodScheduled",
					"status":             "False",
					"reason":             "Unknown",
				},
			},
			expectedConditions: []Condition{
				{
					Type:               ConditionReconciling,
					Status:             corev1.ConditionTrue,
					Reason:             "PodPending",
					LastTransitionTime: metav1.Time{Time: timestamp},
				},
				{
					Type:               "PodScheduled",
					Status:             corev1.ConditionFalse,
					Reason:             "Unknown",
					LastTransitionTime: metav1.Time{Time: timestamp},
				},
			},
			checkLastTransitionTime: true,
		},
		"has other existing conditions": {
			manifest: pod,
			withConditions: []map[string]interface{}{
				{
					"lastTransitionTime": timestampStr,
					"lastUpdateTime":     timestampStr,
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
			withConditions: []map[string]interface{}{
				{
					"lastTransitionTime": timestampStr,
					"lastUpdateTime":     timestampStr,
					"type":               ConditionReconciling.String(),
					"status":             "True",
					"reason":             "PodIsAbsolutelyNotReady",
				},
			},
			expectedConditions: []Condition{
				{
					Type:               ConditionReconciling,
					Status:             corev1.ConditionTrue,
					Reason:             "PodIsAbsolutelyNotReady",
					LastTransitionTime: metav1.Time{Time: timestamp},
				},
			},
			checkLastTransitionTime: true,
		},
		"already has condition of standard type Failed": {
			manifest: pod,
			withConditions: []map[string]interface{}{
				{
					"lastTransitionTime": timestampStr,
					"lastUpdateTime":     timestampStr,
					"type":               ConditionStalled.String(),
					"status":             "True",
					"reason":             "PodHasFailed",
				},
			},
			expectedConditions: []Condition{
				{
					Type:               ConditionStalled,
					Status:             corev1.ConditionTrue,
					Reason:             "PodHasFailed",
					LastTransitionTime: metav1.Time{Time: timestamp},
				},
			},
			checkLastTransitionTime: true,
		},
		"custom resource with no conditions": {
			manifest:           custom,
			withConditions:     []map[string]interface{}{},
			expectedConditions: []Condition{},
		},
	}

	for tn, tc := range testCases {
		tc := tc
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
					assert.Equal(t, expectedCondition.Reason, condition.Reason)
					if tc.checkLastTransitionTime {
						assert.True(t, expectedCondition.LastTransitionTime.Time.Equal(condition.LastTransitionTime.Time))
					}
				}
				assert.True(t, found)
			}
		})
	}
}

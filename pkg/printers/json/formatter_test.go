// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/list"
	"sigs.k8s.io/cli-utils/pkg/print/stats"
)

func TestFormatter_FormatApplyEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.ApplyEvent
		expected        []map[string]interface{}
	}{
		"resource created without dryrun": {
			previewStrategy: common.DryRunNone,
			event: event.ApplyEvent{
				Operation:  event.Created,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: []map[string]interface{}{
				{
					"eventType": "resourceApplied",
					"group":     "apps",
					"kind":      "Deployment",
					"name":      "my-dep",
					"namespace": "default",
					"operation": "Created",
					"timestamp": "",
					"type":      "apply",
				},
			},
		},
		"resource updated with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.ApplyEvent{
				Operation:  event.Configured,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: []map[string]interface{}{
				{
					"eventType": "resourceApplied",
					"group":     "apps",
					"kind":      "Deployment",
					"name":      "my-dep",
					"namespace": "",
					"operation": "Configured",
					"timestamp": "",
					"type":      "apply",
				},
			},
		},
		"resource updated with server dryrun": {
			previewStrategy: common.DryRunServer,
			event: event.ApplyEvent{
				Operation:  event.Configured,
				Identifier: createIdentifier("batch", "CronJob", "foo", "my-cron"),
			},
			expected: []map[string]interface{}{
				{
					"eventType": "resourceApplied",
					"group":     "batch",
					"kind":      "CronJob",
					"name":      "my-cron",
					"namespace": "foo",
					"operation": "Configured",
					"timestamp": "",
					"type":      "apply",
				},
			},
		},
		"resource apply error": {
			previewStrategy: common.DryRunNone,
			event: event.ApplyEvent{
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: []map[string]interface{}{
				{
					"eventType": "resourceFailed",
					"group":     "apps",
					"kind":      "Deployment",
					"name":      "my-dep",
					"namespace": "",
					"timestamp": "",
					"type":      "apply",
					"error":     "example error",
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatApplyEvent(tc.event)
			assert.NoError(t, err)

			objects := strings.Split(strings.TrimSpace(out.String()), "\n")

			if !assert.Equal(t, len(tc.expected), len(objects)) {
				t.FailNow()
			}
			for i := range tc.expected {
				assertOutput(t, tc.expected[i], objects[i])
			}
		})
	}
}

func TestFormatter_FormatStatusEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.StatusEvent
		expected        map[string]interface{}
	}{
		"resource update with Current status": {
			previewStrategy: common.DryRunNone,
			event: event.StatusEvent{
				Identifier: object.ObjMetadata{
					GroupKind: schema.GroupKind{
						Group: "apps",
						Kind:  "Deployment",
					},
					Namespace: "foo",
					Name:      "bar",
				},
				PollResourceInfo: &pollevent.ResourceStatus{
					Identifier: object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Namespace: "foo",
						Name:      "bar",
					},
					Status:  status.CurrentStatus,
					Message: "Resource is Current",
				},
			},
			expected: map[string]interface{}{
				"eventType": "resourceStatus",
				"group":     "apps",
				"kind":      "Deployment",
				"message":   "Resource is Current",
				"name":      "bar",
				"namespace": "foo",
				"status":    "Current",
				"timestamp": "",
				"type":      "status",
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatStatusEvent(tc.event)
			assert.NoError(t, err)

			assertOutput(t, tc.expected, out.String())
		})
	}
}

func TestFormatter_FormatPruneEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.PruneEvent
		expected        map[string]interface{}
	}{
		"resource pruned without dryrun": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Operation:  event.Pruned,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourcePruned",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Pruned",
				"timestamp": "",
				"type":      "prune",
			},
		},
		"resource skipped with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.PruneEvent{
				Operation:  event.PruneSkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourcePruned",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "",
				"operation": "PruneSkipped",
				"timestamp": "",
				"type":      "prune",
			},
		},
		"resource prune error": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceFailed",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "",
				"timestamp": "",
				"type":      "prune",
				"error":     "example error",
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatPruneEvent(tc.event)
			assert.NoError(t, err)

			assertOutput(t, tc.expected, out.String())
		})
	}
}

func TestFormatter_FormatDeleteEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.DeleteEvent
		statusCollector list.Collector
		expected        map[string]interface{}
	}{
		"resource deleted without no dryrun": {
			previewStrategy: common.DryRunNone,
			event: event.DeleteEvent{
				Operation:  event.Deleted,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceDeleted",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Deleted",
				"timestamp": "",
				"type":      "delete",
			},
		},
		"resource skipped with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.DeleteEvent{
				Operation:  event.DeleteSkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceDeleted",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "",
				"operation": "DeleteSkipped",
				"timestamp": "",
				"type":      "delete",
			},
		},
		"resource delete error": {
			previewStrategy: common.DryRunNone,
			event: event.DeleteEvent{
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceFailed",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"timestamp": "",
				"type":      "delete",
				"error":     "example error",
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatDeleteEvent(tc.event)
			assert.NoError(t, err)

			assertOutput(t, tc.expected, out.String())
		})
	}
}

func TestFormatter_FormatWaitEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.WaitEvent
		statusCollector list.Collector
		expected        map[string]interface{}
	}{
		"resource reconciled": {
			previewStrategy: common.DryRunNone,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceReconciled",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Reconciled",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconciled (client-side dry-run)": {
			previewStrategy: common.DryRunClient,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceReconciled",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Reconciled",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconciled (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceReconciled",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Reconciled",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile pending": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcilePending,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceReconciled",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Pending",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile skipped": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileSkipped,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceReconciled",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Skipped",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile timeout": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileTimeout,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceReconciled",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Timeout",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile failed": {
			previewStrategy: common.DryRunNone,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileFailed,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"eventType": "resourceReconciled",
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"operation": "Failed",
				"timestamp": "",
				"type":      "wait",
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatWaitEvent(tc.event)
			assert.NoError(t, err)

			assertOutput(t, tc.expected, out.String())
		})
	}
}

func TestFormatter_FormatActionGroupEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.ActionGroupEvent
		actionGroups    []event.ActionGroup
		statsCollector  stats.Stats
		statusCollector list.Collector
		expected        map[string]interface{}
	}{
		"not the last apply action group finished": {
			previewStrategy: common.DryRunNone,
			event: event.ActionGroupEvent{
				GroupName: "age-1",
				Action:    event.ApplyAction,
				Type:      event.Finished,
			},
			actionGroups: []event.ActionGroup{
				{
					Name:   "age-1",
					Action: event.ApplyAction,
				},
				{
					Name:   "age-2",
					Action: event.ApplyAction,
				},
			},
			expected: map[string]interface{}{},
		},
		"the last apply action group finished": {
			previewStrategy: common.DryRunNone,
			event: event.ActionGroupEvent{
				GroupName: "age-2",
				Action:    event.ApplyAction,
				Type:      event.Finished,
			},
			actionGroups: []event.ActionGroup{
				{
					Name:   "age-1",
					Action: event.ApplyAction,
				},
				{
					Name:   "age-2",
					Action: event.ApplyAction,
				},
			},
			statsCollector: stats.Stats{
				ApplyStats: stats.ApplyStats{
					ServersideApplied: 42,
				},
			},
			expected: map[string]interface{}{
				"eventType":       "completed",
				"configuredCount": 0,
				"count":           42,
				"createdCount":    0,
				"failedCount":     0,
				"serverSideCount": 42,
				"timestamp":       "2022-01-06T05:22:48Z",
				"type":            "apply",
				"unchangedCount":  0,
			},
		},
		"last prune action group started": {
			previewStrategy: common.DryRunNone,
			event: event.ActionGroupEvent{
				GroupName: "age-2",
				Action:    event.PruneAction,
				Type:      event.Started,
			},
			actionGroups: []event.ActionGroup{
				{
					Name:   "age-1",
					Action: event.PruneAction,
				},
				{
					Name:   "age-2",
					Action: event.PruneAction,
				},
			},
			expected: map[string]interface{}{},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatActionGroupEvent(tc.event, tc.actionGroups, tc.statsCollector, tc.statusCollector)
			assert.NoError(t, err)

			assertOutput(t, tc.expected, out.String())
		})
	}
}

// nolint:unparam
func assertOutput(t *testing.T, expectedMap map[string]interface{}, actual string) bool {
	if len(expectedMap) == 0 {
		return assert.Empty(t, actual)
	}

	var m map[string]interface{}
	err := json.Unmarshal([]byte(actual), &m)
	if !assert.NoError(t, err) {
		return false
	}

	if _, found := expectedMap["timestamp"]; found {
		if _, ok := m["timestamp"]; ok {
			delete(expectedMap, "timestamp")
			delete(m, "timestamp")
		} else {
			t.Error("expected to find key 'timestamp', but didn't")
			return false
		}
	}

	for key, val := range m {
		if floatVal, ok := val.(float64); ok {
			m[key] = int(floatVal)
		}
	}

	return assert.Equal(t, expectedMap, m)
}

func createIdentifier(group, kind, namespace, name string) object.ObjMetadata {
	return object.ObjMetadata{
		Namespace: namespace,
		Name:      name,
		GroupKind: schema.GroupKind{
			Group: group,
			Kind:  kind,
		},
	}
}

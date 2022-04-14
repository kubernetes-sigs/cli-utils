// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/graph"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/print/list"
	"sigs.k8s.io/cli-utils/pkg/print/stats"
	"sigs.k8s.io/cli-utils/pkg/testutil"
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
				Status:     event.ApplySuccessful,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: []map[string]interface{}{
				{
					"group":     "apps",
					"kind":      "Deployment",
					"name":      "my-dep",
					"namespace": "default",
					"status":    "Successful",
					"timestamp": "",
					"type":      "apply",
				},
			},
		},
		"resource updated with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.ApplyEvent{
				Status:     event.ApplySuccessful,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: []map[string]interface{}{
				{
					"group":     "apps",
					"kind":      "Deployment",
					"name":      "my-dep",
					"namespace": "",
					"status":    "Successful",
					"timestamp": "",
					"type":      "apply",
				},
			},
		},
		"resource updated with server dryrun": {
			previewStrategy: common.DryRunServer,
			event: event.ApplyEvent{
				Status:     event.ApplySuccessful,
				Identifier: createIdentifier("batch", "CronJob", "foo", "my-cron"),
			},
			expected: []map[string]interface{}{
				{
					"group":     "batch",
					"kind":      "CronJob",
					"name":      "my-cron",
					"namespace": "foo",
					"status":    "Successful",
					"timestamp": "",
					"type":      "apply",
				},
			},
		},
		"resource apply failed": {
			previewStrategy: common.DryRunNone,
			event: event.ApplyEvent{
				Status:     event.ApplyFailed,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: []map[string]interface{}{
				{
					"group":     "apps",
					"kind":      "Deployment",
					"name":      "my-dep",
					"namespace": "",
					"status":    "Failed",
					"timestamp": "",
					"type":      "apply",
					"error":     "example error",
				},
			},
		},
		"resource apply skip error": {
			previewStrategy: common.DryRunNone,
			event: event.ApplyEvent{
				Status:     event.ApplySkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: []map[string]interface{}{
				{
					"group":     "apps",
					"kind":      "Deployment",
					"name":      "my-dep",
					"namespace": "",
					"status":    "Skipped",
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
				Status:     event.PruneSuccessful,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Successful",
				"timestamp": "",
				"type":      "prune",
			},
		},
		"resource skipped with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.PruneEvent{
				Status:     event.PruneSkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "",
				"status":    "Skipped",
				"timestamp": "",
				"type":      "prune",
			},
		},
		"resource prune failed": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Status:     event.PruneFailed,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "",
				"status":    "Failed",
				"timestamp": "",
				"type":      "prune",
				"error":     "example error",
			},
		},
		"resource prune skip error": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Status:     event.PruneSkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "",
				"status":    "Skipped",
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
				Status:     event.DeleteSuccessful,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Successful",
				"timestamp": "",
				"type":      "delete",
			},
		},
		"resource skipped with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.DeleteEvent{
				Status:     event.DeleteSkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "",
				"status":    "Skipped",
				"timestamp": "",
				"type":      "delete",
			},
		},
		"resource delete failed": {
			previewStrategy: common.DryRunNone,
			event: event.DeleteEvent{
				Status:     event.DeleteFailed,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Failed",
				"timestamp": "",
				"type":      "delete",
				"error":     "example error",
			},
		},
		"resource delete skip error": {
			previewStrategy: common.DryRunNone,
			event: event.DeleteEvent{
				Status:     event.DeleteSkipped,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Error:      errors.New("example error"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Skipped",
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
				Status:     event.ReconcileSuccessful,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Successful",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconciled (client-side dry-run)": {
			previewStrategy: common.DryRunClient,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileSuccessful,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Successful",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconciled (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileSuccessful,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Successful",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile pending": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcilePending,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Pending",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile skipped": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileSkipped,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Skipped",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile timeout": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileTimeout,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Timeout",
				"timestamp": "",
				"type":      "wait",
			},
		},
		"resource reconcile failed": {
			previewStrategy: common.DryRunNone,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Status:     event.ReconcileFailed,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: map[string]interface{}{
				"group":     "apps",
				"kind":      "Deployment",
				"name":      "my-dep",
				"namespace": "default",
				"status":    "Failed",
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
				Status:    event.Finished,
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
				ApplyStats: stats.ApplyStats{},
			},
			expected: map[string]interface{}{
				"action":     "Apply",
				"count":      0,
				"failed":     0,
				"skipped":    0,
				"status":     "Finished",
				"successful": 0,
				"timestamp":  "2022-03-24T01:35:04Z",
				"type":       "group",
			},
		},
		"the last apply action group finished": {
			previewStrategy: common.DryRunNone,
			event: event.ActionGroupEvent{
				GroupName: "age-2",
				Action:    event.ApplyAction,
				Status:    event.Finished,
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
					Successful: 42,
				},
			},
			expected: map[string]interface{}{
				"action":     "Apply",
				"count":      42,
				"failed":     0,
				"skipped":    0,
				"status":     "Finished",
				"successful": 42,
				"timestamp":  "2022-03-24T01:35:04Z",
				"type":       "group",
			},
		},
		"last prune action group started": {
			previewStrategy: common.DryRunNone,
			event: event.ActionGroupEvent{
				GroupName: "age-2",
				Action:    event.PruneAction,
				Status:    event.Started,
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
			expected: map[string]interface{}{
				"action":    "Prune",
				"status":    "Started",
				"timestamp": "2022-03-24T01:51:36Z",
				"type":      "group",
			},
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

func TestFormatter_FormatValidationEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.ValidationEvent
		expected        map[string]interface{}
		expectedError   error
	}{
		"zero objects, return error": {
			previewStrategy: common.DryRunNone,
			event: event.ValidationEvent{
				Identifiers: object.ObjMetadataSet{},
				Error:       errors.New("unexpected"),
			},
			expectedError: errors.New("invalid validation event: no identifiers: unexpected"),
		},
		"one object, missing namespace": {
			previewStrategy: common.DryRunNone,
			event: event.ValidationEvent{
				Identifiers: object.ObjMetadataSet{
					{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Namespace: "foo",
						Name:      "bar",
					},
				},
				Error: validation.NewError(
					field.Required(field.NewPath("metadata", "namespace"), "namespace is required"),
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Namespace: "foo",
						Name:      "bar",
					},
				),
			},
			expected: map[string]interface{}{
				"type":      "validation",
				"timestamp": "",
				"objects": []interface{}{
					map[string]interface{}{
						"group":     "apps",
						"kind":      "Deployment",
						"name":      "bar",
						"namespace": "foo",
					},
				},
				"error": "metadata.namespace: Required value: namespace is required",
			},
		},
		"two objects, cyclic dependency": {
			previewStrategy: common.DryRunNone,
			event: event.ValidationEvent{
				Identifiers: object.ObjMetadataSet{
					{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Namespace: "default",
						Name:      "bar",
					},
					{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Namespace: "default",
						Name:      "foo",
					},
				},
				Error: validation.NewError(
					graph.CyclicDependencyError{
						Edges: []graph.Edge{
							{
								From: object.ObjMetadata{
									GroupKind: schema.GroupKind{
										Group: "apps",
										Kind:  "Deployment",
									},
									Namespace: "default",
									Name:      "bar",
								},
								To: object.ObjMetadata{
									GroupKind: schema.GroupKind{
										Group: "apps",
										Kind:  "Deployment",
									},
									Namespace: "default",
									Name:      "foo",
								},
							},
							{
								From: object.ObjMetadata{
									GroupKind: schema.GroupKind{
										Group: "apps",
										Kind:  "Deployment",
									},
									Namespace: "default",
									Name:      "foo",
								},
								To: object.ObjMetadata{
									GroupKind: schema.GroupKind{
										Group: "apps",
										Kind:  "Deployment",
									},
									Namespace: "default",
									Name:      "bar",
								},
							},
						},
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Namespace: "default",
						Name:      "bar",
					},
					object.ObjMetadata{
						GroupKind: schema.GroupKind{
							Group: "apps",
							Kind:  "Deployment",
						},
						Namespace: "default",
						Name:      "foo",
					},
				),
			},
			expected: map[string]interface{}{
				"type":      "validation",
				"timestamp": "",
				"objects": []interface{}{
					map[string]interface{}{
						"group":     "apps",
						"kind":      "Deployment",
						"name":      "bar",
						"namespace": "default",
					},
					map[string]interface{}{
						"group":     "apps",
						"kind":      "Deployment",
						"name":      "foo",
						"namespace": "default",
					},
				},
				"error": `cyclic dependency:
- apps/namespaces/default/Deployment/bar -> apps/namespaces/default/Deployment/foo
- apps/namespaces/default/Deployment/foo -> apps/namespaces/default/Deployment/bar`,
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatValidationEvent(tc.event)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			assertOutput(t, tc.expected, out.String())
		})
	}
}

func TestFormatter_FormatSummary(t *testing.T) {
	now := time.Now()
	nowStr := now.UTC().Format(time.RFC3339)

	testCases := map[string]struct {
		statsCollector stats.Stats
		expected       []map[string]interface{}
	}{
		"apply prune wait": {
			statsCollector: stats.Stats{
				ApplyStats: stats.ApplyStats{
					Successful: 1,
					Skipped:    2,
					Failed:     3,
				},
				PruneStats: stats.PruneStats{
					Successful: 3,
					Skipped:    2,
					Failed:     1,
				},
				WaitStats: stats.WaitStats{
					Successful: 4,
					Skipped:    6,
					Failed:     1,
					Timeout:    1,
				},
			},
			expected: []map[string]interface{}{
				{
					"action":     "Apply",
					"count":      float64(6),
					"successful": float64(1),
					"skipped":    float64(2),
					"failed":     float64(3),
					"timestamp":  nowStr,
					"type":       "summary",
				},
				{
					"action":     "Prune",
					"count":      float64(6),
					"successful": float64(3),
					"skipped":    float64(2),
					"failed":     float64(1),
					"timestamp":  nowStr,
					"type":       "summary",
				},
				{
					"action":     "Wait",
					"count":      float64(12),
					"successful": float64(4),
					"skipped":    float64(6),
					"failed":     float64(1),
					"timeout":    float64(1),
					"timestamp":  nowStr,
					"type":       "summary",
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			jf := &formatter{
				ioStreams: ioStreams,
				// fake time func
				now: func() time.Time { return now },
			}
			err := jf.FormatSummary(tc.statsCollector)
			assert.NoError(t, err)

			assertOutputLines(t, tc.expected, out.String())
		})
	}
}

func assertOutputLines(t *testing.T, expectedMaps []map[string]interface{}, actual string) {
	actual = strings.TrimRight(actual, "\n")
	lines := strings.Split(actual, "\n")
	actualMaps := make([]map[string]interface{}, len(lines))
	for i, line := range lines {
		err := json.Unmarshal([]byte(line), &actualMaps[i])
		require.NoError(t, err)
	}
	testutil.AssertEqual(t, expectedMaps, actualMaps)
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

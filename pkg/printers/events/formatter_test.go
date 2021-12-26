// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/list"
	"sigs.k8s.io/cli-utils/pkg/printers/printer"
)

func TestFormatter_FormatApplyEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.ApplyEvent
		applyStats      *list.ApplyStats
		statusCollector list.Collector
		expected        string
	}{
		"resource created without no dryrun": {
			previewStrategy: common.DryRunNone,
			event: event.ApplyEvent{
				Operation:  event.Created,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep created",
		},
		"resource updated with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.ApplyEvent{
				Operation:  event.Configured,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: "deployment.apps/my-dep configured (preview)",
		},
		"resource updated with server dryrun": {
			previewStrategy: common.DryRunServer,
			event: event.ApplyEvent{
				Operation:  event.Configured,
				Identifier: createIdentifier("batch", "CronJob", "foo", "my-cron"),
			},
			expected: "cronjob.batch/my-cron configured (preview-server)",
		},
		"apply event with error should display the error": {
			previewStrategy: common.DryRunServer,
			event: event.ApplyEvent{
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      fmt.Errorf("this is a test error"),
			},
			expected: "deployment.apps/my-dep apply failed: this is a test error (preview-server)",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy, printer.PreviewStringer{})
			err := formatter.FormatApplyEvent(tc.event)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
}

func TestFormatter_FormatStatusEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.StatusEvent
		statusCollector list.Collector
		expected        string
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
			expected: "deployment.apps/bar is Current: Resource is Current",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy, printer.PreviewStringer{})
			err := formatter.FormatStatusEvent(tc.event)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
}

func TestFormatter_FormatPruneEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.PruneEvent
		pruneStats      *list.PruneStats
		expected        string
	}{
		"resource pruned without no dryrun": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Operation:  event.Pruned,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep pruned",
		},
		"resource skipped with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.PruneEvent{
				Operation:  event.PruneSkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: "deployment.apps/my-dep prune skipped (preview)",
		},
		"resource with prune error": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      fmt.Errorf("this is a test"),
			},
			expected: "deployment.apps/my-dep prune failed: this is a test",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy, printer.PreviewStringer{})
			err := formatter.FormatPruneEvent(tc.event)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
}

func TestFormatter_FormatDeleteEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.DeleteEvent
		deleteStats     *list.DeleteStats
		statusCollector list.Collector
		expected        string
	}{
		"resource deleted without no dryrun": {
			previewStrategy: common.DryRunNone,
			event: event.DeleteEvent{
				Operation:  event.Deleted,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Object:     createObject("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep deleted",
		},
		"resource skipped with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.DeleteEvent{
				Operation:  event.DeleteSkipped,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Object:     createObject("apps", "Deployment", "", "my-dep"),
			},
			expected: "deployment.apps/my-dep delete skipped (preview)",
		},
		"resource with delete error": {
			previewStrategy: common.DryRunServer,
			event: event.DeleteEvent{
				Object:     createObject("apps", "Deployment", "", "my-dep"),
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      fmt.Errorf("this is a test"),
			},
			expected: "deployment.apps/my-dep deletion failed: this is a test (preview-server)",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy, printer.PreviewStringer{})
			err := formatter.FormatDeleteEvent(tc.event)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
}

func TestFormatter_FormatWaitEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.WaitEvent
		waitStats       *list.WaitStats
		statusCollector list.Collector
		expected        string
	}{
		"resource reconciled": {
			previewStrategy: common.DryRunNone,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconciled",
		},
		"resource reconciled (client-side dry-run)": {
			previewStrategy: common.DryRunClient,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconciled (preview)",
		},
		"resource reconciled (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconciled (preview-server)",
		},
		"resource reconcile timeout": {
			previewStrategy: common.DryRunNone,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Operation:  event.ReconcileTimeout,
			},
			expected: "deployment.apps/my-dep reconcile timeout",
		},
		"resource reconcile timeout (client-side dry-run)": {
			previewStrategy: common.DryRunClient,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Operation:  event.ReconcileTimeout,
			},
			expected: "deployment.apps/my-dep reconcile timeout (preview)",
		},
		"resource reconcile timeout (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Operation:  event.ReconcileTimeout,
			},
			expected: "deployment.apps/my-dep reconcile timeout (preview-server)",
		},
		"resource reconcile skipped": {
			previewStrategy: common.DryRunNone,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileSkipped,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconcile skipped",
		},
		"resource reconcile skipped (client-side dry-run)": {
			previewStrategy: common.DryRunClient,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileSkipped,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconcile skipped (preview)",
		},
		"resource reconcile skipped (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileSkipped,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconcile skipped (preview-server)",
		},
		"resource reconcile failed": {
			previewStrategy: common.DryRunNone,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileFailed,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconcile failed",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy, printer.PreviewStringer{})
			err := formatter.FormatWaitEvent(tc.event)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
}

func TestCustomDryRunStringer(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		format          func(formatter list.Formatter) error
		expected        string
	}{
		"Apply event with DryRunServer": {
			previewStrategy: common.DryRunServer,
			format: func(formatter list.Formatter) error {
				return formatter.FormatApplyEvent(event.ApplyEvent{
					Operation:  event.Configured,
					Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				})
			},
			expected: "deployment.apps/my-dep configured -- DRYRUNSERVER",
		},
		"Prune event with DryRunNone": {
			previewStrategy: common.DryRunNone,
			format: func(formatter list.Formatter) error {
				return formatter.FormatPruneEvent(event.PruneEvent{
					Operation:  event.Pruned,
					Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				})
			},
			expected: "deployment.apps/my-dep pruned",
		},
		"Wait event with DryRunClient": {
			previewStrategy: common.DryRunClient,
			format: func(formatter list.Formatter) error {
				return formatter.FormatWaitEvent(event.WaitEvent{
					Operation:  event.ReconcileFailed,
					Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				})
			},
			expected: "deployment.apps/my-dep reconcile failed -- DRYRUNCLIENT",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy, allCapsStringer{})
			err := tc.format(formatter)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
}

type allCapsStringer struct{}

func (a allCapsStringer) String(strategy common.DryRunStrategy) string {
	if strategy.ClientOrServerDryRun() {
		return fmt.Sprintf(" -- %s", strings.ToUpper(strategy.String()))
	}
	return ""
}

func createObject(group, kind, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/v1", group),
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
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

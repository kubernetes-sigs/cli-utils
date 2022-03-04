// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
)

func TestFormatter_FormatApplyEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.ApplyEvent
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
			expected: "deployment.apps/my-dep configured",
		},
		"resource updated with server dryrun": {
			previewStrategy: common.DryRunServer,
			event: event.ApplyEvent{
				Operation:  event.Configured,
				Identifier: createIdentifier("batch", "CronJob", "foo", "my-cron"),
			},
			expected: "cronjob.batch/my-cron configured",
		},
		"apply event with error should display the error": {
			previewStrategy: common.DryRunServer,
			event: event.ApplyEvent{
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      fmt.Errorf("this is a test error"),
			},
			expected: "deployment.apps/my-dep apply failed: this is a test error",
		},
		"apply event with skip error should display the error": {
			previewStrategy: common.DryRunServer,
			event: event.ApplyEvent{
				Operation:  event.Unchanged,
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      fmt.Errorf("this is a test error"),
			},
			expected: "deployment.apps/my-dep apply skipped: this is a test error",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
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
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
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
		expected        string
	}{
		"resource pruned without no dryrun": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Operation:  event.Pruned,
				Object:     createObject("apps", "Deployment", "", "my-dep"),
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep pruned",
		},
		"resource skipped with client dryrun": {
			previewStrategy: common.DryRunClient,
			event: event.PruneEvent{
				Operation:  event.PruneSkipped,
				Object:     createObject("apps", "Deployment", "", "my-dep"),
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
			},
			expected: "deployment.apps/my-dep prune skipped",
		},
		"resource with prune error": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Object:     createObject("apps", "Deployment", "", "my-dep"),
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      fmt.Errorf("this is a test"),
			},
			expected: "deployment.apps/my-dep prune failed: this is a test",
		},

		"resource with prune skip error": {
			previewStrategy: common.DryRunNone,
			event: event.PruneEvent{
				Operation:  event.PruneSkipped,
				Object:     createObject("batch", "CronJob", "foo", "my-cron"),
				Identifier: createIdentifier("batch", "CronJob", "foo", "my-cron"),
				Error:      fmt.Errorf("this is a test"),
			},
			expected: "cronjob.batch/my-cron prune skipped: this is a test",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
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
			expected: "deployment.apps/my-dep delete skipped",
		},
		"resource with delete error": {
			previewStrategy: common.DryRunServer,
			event: event.DeleteEvent{
				Object:     createObject("apps", "Deployment", "", "my-dep"),
				Identifier: createIdentifier("apps", "Deployment", "", "my-dep"),
				Error:      fmt.Errorf("this is a test"),
			},
			expected: "deployment.apps/my-dep delete failed: this is a test",
		},
		"resource with delete skip error": {
			previewStrategy: common.DryRunServer,
			event: event.DeleteEvent{
				Operation:  event.DeleteSkipped,
				Object:     createObject("batch", "CronJob", "foo", "my-cron"),
				Identifier: createIdentifier("batch", "CronJob", "foo", "my-cron"),
				Error:      fmt.Errorf("this is a test"),
			},
			expected: "cronjob.batch/my-cron delete skipped: this is a test",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
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
			expected: "deployment.apps/my-dep reconciled",
		},
		"resource reconciled (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconciled",
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
			expected: "deployment.apps/my-dep reconcile timeout",
		},
		"resource reconcile timeout (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
				Operation:  event.ReconcileTimeout,
			},
			expected: "deployment.apps/my-dep reconcile timeout",
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
			expected: "deployment.apps/my-dep reconcile skipped",
		},
		"resource reconcile skipped (server-side dry-run)": {
			previewStrategy: common.DryRunServer,
			event: event.WaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcileSkipped,
				Identifier: createIdentifier("apps", "Deployment", "default", "my-dep"),
			},
			expected: "deployment.apps/my-dep reconcile skipped",
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
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatWaitEvent(tc.event)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
}

func TestFormatter_FormatValidationEvent(t *testing.T) {
	testCases := map[string]struct {
		previewStrategy common.DryRunStrategy
		event           event.ValidationEvent
		statusCollector list.Collector
		expected        string
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
			expected: "Invalid object (deployment.apps/bar): metadata.namespace: Required value: namespace is required",
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
			expected: `Invalid objects (deployment.apps/bar, deployment.apps/foo): cyclic dependency:
- apps/namespaces/default/Deployment/bar -> apps/namespaces/default/Deployment/foo
- apps/namespaces/default/Deployment/foo -> apps/namespaces/default/Deployment/bar`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			ioStreams, _, out, _ := genericclioptions.NewTestIOStreams() //nolint:dogsled
			formatter := NewFormatter(ioStreams, tc.previewStrategy)
			err := formatter.FormatValidationEvent(tc.event)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expected, strings.TrimSpace(out.String()))
		})
	}
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

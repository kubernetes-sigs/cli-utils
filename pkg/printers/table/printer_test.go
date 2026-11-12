// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package table

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	pe "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/table"
	"sigs.k8s.io/cli-utils/pkg/printers/printer"
	printertesting "sigs.k8s.io/cli-utils/pkg/printers/testutil"
)

func TestActionColumnDef(t *testing.T) {
	testCases := map[string]struct {
		resource       table.Resource
		columnWidth    int
		expectedOutput string
	}{
		"unexpected implementation of Resource interface": {
			resource:       &subResourceInfo{},
			columnWidth:    15,
			expectedOutput: "",
		},
		"neither applied nor pruned": {
			resource:       &resourceInfo{},
			columnWidth:    15,
			expectedOutput: "Pending",
		},
		"applied": {
			resource: &resourceInfo{
				ResourceAction: event.ApplyAction,
				ApplyStatus:    event.ApplySuccessful,
			},
			columnWidth:    15,
			expectedOutput: "Successful",
		},
		"pruned": {
			resource: &resourceInfo{
				ResourceAction: event.PruneAction,
				PruneStatus:    event.PruneSuccessful,
			},
			columnWidth:    15,
			expectedOutput: "Successful",
		},
		"trimmed output": {
			resource: &resourceInfo{
				ResourceAction: event.ApplyAction,
				ApplyStatus:    event.ApplySuccessful,
			},
			columnWidth:    5,
			expectedOutput: "Succe",
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			var buf bytes.Buffer
			_, err := actionColumnDef.PrintResource(&buf, tc.columnWidth, tc.resource)
			if err != nil {
				t.Error(err)
			}

			if want, got := tc.expectedOutput, buf.String(); want != got {
				t.Errorf("expected %q, but got %q", want, got)
			}
		})
	}
}

func TestPrint(t *testing.T) {
	printertesting.PrintResultErrorTest(t, func() printer.Printer {
		ioStreams, _, _, _ := genericiooptions.NewTestIOStreams()
		return &Printer{
			IOStreams: ioStreams,
		}
	})
}

func TestTableMessagePreservesActuationError(t *testing.T) {
	applyError := errors.New("spec.ports: Required value")
	pruneError := errors.New("prune request forbidden")
	deleteError := errors.New("delete request forbidden")
	tests := []struct {
		name      string
		action    event.ResourceAction
		event     event.Event
		pollError error
		want      string
	}{
		{
			name:   "apply failure",
			action: event.ApplyAction,
			event: event.Event{Type: event.ApplyType, ApplyEvent: event.ApplyEvent{
				Identifier: depID, Status: event.ApplyFailed, Error: applyError,
			}},
			want: applyError.Error(),
		},
		{
			name:   "prune failure",
			action: event.PruneAction,
			event: event.Event{Type: event.PruneType, PruneEvent: event.PruneEvent{
				Identifier: depID, Status: event.PruneFailed, Error: pruneError,
			}},
			want: pruneError.Error(),
		},
		{
			name:   "delete failure",
			action: event.DeleteAction,
			event: event.Event{Type: event.DeleteType, DeleteEvent: event.DeleteEvent{
				Identifier: depID, Status: event.DeleteFailed, Error: deleteError,
			}},
			want: deleteError.Error(),
		},
		{
			name:   "successful apply shows resource status",
			action: event.ApplyAction,
			event: event.Event{Type: event.ApplyType, ApplyEvent: event.ApplyEvent{
				Identifier: depID, Status: event.ApplySuccessful,
			}},
			want: "Resource not found",
		},
		{
			name:   "successful apply shows polling error",
			action: event.ApplyAction,
			event: event.Event{Type: event.ApplyType, ApplyEvent: event.ApplyEvent{
				Identifier: depID, Status: event.ApplySuccessful,
			}},
			pollError: errors.New("poller unavailable"),
			want:      "poller unavailable",
		},
	}
	for _, tt := range tests {
		for _, order := range []string{"status-first", "status-last"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				collector := newResourceStateCollector([]event.ActionGroup{{
					Action: tt.action, Identifiers: object.ObjMetadataSet{depID},
				}})
				observed := &pe.ResourceStatus{
					Identifier: depID, Status: status.NotFoundStatus,
					Message: "Resource not found", Error: tt.pollError,
					GeneratedResources: []*pe.ResourceStatus{{
						Identifier: depID2, Status: status.CurrentStatus,
						Message: "child status",
					}},
				}
				statusEvent := event.Event{Type: event.StatusType, StatusEvent: event.StatusEvent{
					Identifier: depID, PollResourceInfo: observed,
				}}
				events := []event.Event{tt.event, statusEvent}
				if order == "status-first" {
					events = []event.Event{statusEvent, tt.event}
				}
				for _, ev := range events {
					require.NoError(t, collector.processEvent(ev))
				}

				streams, _, output, _ := genericiooptions.NewTestIOStreams()
				printer := table.BaseTablePrinter{IOStreams: streams, Columns: columns}
				printer.PrintTable(collector.LatestState(), 0)
				assert.Contains(t, output.String(), tt.want)
				assert.Contains(t, output.String(), "child status")
				if tt.want != observed.Message {
					assert.NotContains(t, output.String(), observed.Message)
				}
				assert.Equal(t, tt.pollError, observed.Error, "printing must not change the observed resource status")
			})
		}
	}
}

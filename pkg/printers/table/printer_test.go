// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package table

import (
	"bytes"
	"testing"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
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

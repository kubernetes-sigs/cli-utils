// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/manifestreader"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var (
	inventoryTemplate = `
kind: ConfigMap
apiVersion: v1
metadata:
  labels:
    cli-utils.sigs.k8s.io/inventory-id: test
  name: foo
  namespace: default
`
	depObject = object.ObjMetadata{
		Name:      "foo",
		Namespace: "default",
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
	}

	stsObject = object.ObjMetadata{
		Name:      "bar",
		Namespace: "default",
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "StatefulSet",
		},
	}
)

type fakePoller struct {
	events []pollevent.Event
}

func (f *fakePoller) Poll(ctx context.Context, _ object.ObjMetadataSet,
	_ polling.PollOptions) <-chan pollevent.Event {
	eventChannel := make(chan pollevent.Event)
	go func() {
		defer close(eventChannel)
		for _, e := range f.events {
			eventChannel <- e
		}
		<-ctx.Done()
	}()
	return eventChannel
}

func TestCommand(t *testing.T) {
	testCases := map[string]struct {
		pollUntil      string
		printer        string
		timeout        time.Duration
		input          string
		inventory      object.ObjMetadataSet
		events         []pollevent.Event
		expectedErrMsg string
		expectedOutput string
	}{
		"no inventory template": {
			pollUntil:      "known",
			printer:        "events",
			input:          "",
			expectedErrMsg: "Package uninitialized. Please run \"init\" command.",
		},
		"no inventory in live state": {
			pollUntil:      "known",
			printer:        "events",
			input:          inventoryTemplate,
			expectedOutput: "no resources found in the inventory\n",
		},
		"wait for all known": {
			pollUntil: "known",
			printer:   "events",
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.CurrentStatus,
						Message:    "current",
					},
				},
			},
			expectedOutput: `
foo/deployment.apps/default/foo is InProgress: inProgress
foo/statefulset.apps/default/bar is Current: current
`,
		},
		"wait for all current": {
			pollUntil: "current",
			printer:   "events",
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.CurrentStatus,
						Message:    "current",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.CurrentStatus,
						Message:    "current",
					},
				},
			},
			expectedOutput: `
foo/deployment.apps/default/foo is InProgress: inProgress
foo/statefulset.apps/default/bar is InProgress: inProgress
foo/statefulset.apps/default/bar is Current: current
foo/deployment.apps/default/foo is Current: current
`,
		},
		"wait for all deleted": {
			pollUntil: "deleted",
			printer:   "events",
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.NotFoundStatus,
						Message:    "notFound",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.NotFoundStatus,
						Message:    "notFound",
					},
				},
			},
			expectedOutput: `
foo/statefulset.apps/default/bar is NotFound: notFound
foo/deployment.apps/default/foo is NotFound: notFound
`,
		},
		"forever with timeout": {
			pollUntil: "forever",
			printer:   "events",
			timeout:   2 * time.Second,
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
			},
			expectedOutput: `
foo/statefulset.apps/default/bar is InProgress: inProgress
foo/deployment.apps/default/foo is InProgress: inProgress
`,
		},
	}

	jsonTestCases := map[string]struct {
		pollUntil      string
		printer        string
		timeout        time.Duration
		input          string
		inventory      object.ObjMetadataSet
		events         []pollevent.Event
		expectedErrMsg string
		expectedOutput []map[string]interface{}
	}{
		"wait for all known json": {
			pollUntil: "known",
			printer:   "json",
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.CurrentStatus,
						Message:    "current",
					},
				},
			},
			expectedOutput: []map[string]interface{}{
				{
					"group":          "apps",
					"kind":           "Deployment",
					"namespace":      "default",
					"name":           "foo",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "InProgress",
					"message":        "inProgress",
				},
				{
					"group":          "apps",
					"kind":           "StatefulSet",
					"namespace":      "default",
					"name":           "bar",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "Current",
					"message":        "current",
				},
			},
		},
		"wait for all current json": {
			pollUntil: "current",
			printer:   "json",
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.CurrentStatus,
						Message:    "current",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.CurrentStatus,
						Message:    "current",
					},
				},
			},
			expectedOutput: []map[string]interface{}{
				{
					"group":          "apps",
					"kind":           "Deployment",
					"namespace":      "default",
					"name":           "foo",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "InProgress",
					"message":        "inProgress",
				},
				{
					"group":          "apps",
					"kind":           "StatefulSet",
					"namespace":      "default",
					"name":           "bar",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "InProgress",
					"message":        "inProgress",
				},
				{
					"group":          "apps",
					"kind":           "StatefulSet",
					"namespace":      "default",
					"name":           "bar",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "Current",
					"message":        "current",
				},
				{
					"group":          "apps",
					"kind":           "Deployment",
					"namespace":      "default",
					"name":           "foo",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "Current",
					"message":        "current",
				},
			},
		},
		"wait for all deleted json": {
			pollUntil: "deleted",
			printer:   "json",
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.NotFoundStatus,
						Message:    "notFound",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.NotFoundStatus,
						Message:    "notFound",
					},
				},
			},
			expectedOutput: []map[string]interface{}{
				{
					"group":          "apps",
					"kind":           "StatefulSet",
					"namespace":      "default",
					"name":           "bar",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "NotFound",
					"message":        "notFound",
				},
				{
					"group":          "apps",
					"kind":           "Deployment",
					"namespace":      "default",
					"name":           "foo",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "NotFound",
					"message":        "notFound",
				},
			},
		},
		"forever with timeout json": {
			pollUntil: "forever",
			printer:   "json",
			timeout:   2 * time.Second,
			input:     inventoryTemplate,
			inventory: object.ObjMetadataSet{
				depObject,
				stsObject,
			},
			events: []pollevent.Event{
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: stsObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
				{
					Type: pollevent.ResourceUpdateEvent,
					Resource: &pollevent.ResourceStatus{
						Identifier: depObject,
						Status:     status.InProgressStatus,
						Message:    "inProgress",
					},
				},
			},
			expectedOutput: []map[string]interface{}{
				{
					"group":          "apps",
					"kind":           "StatefulSet",
					"namespace":      "default",
					"name":           "bar",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "InProgress",
					"message":        "inProgress",
				},
				{
					"group":          "apps",
					"kind":           "Deployment",
					"namespace":      "default",
					"name":           "foo",
					"timestamp":      "",
					"type":           "status",
					"inventory-name": "foo",
					"status":         "InProgress",
					"message":        "inProgress",
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("namespace")
			defer tf.Cleanup()

			loader := manifestreader.NewFakeLoader(tf, tc.inventory)
			runner := &Runner{
				factory:    tf,
				invFactory: inventory.FakeClientFactory(tc.inventory),
				loader:     NewInventoryLoader(loader),
				PollerFactoryFunc: func(c cmdutil.Factory) (poller.Poller, error) {
					return &fakePoller{tc.events}, nil
				},

				pollUntil: tc.pollUntil,
				output:    tc.printer,
				timeout:   tc.timeout,
				invType:   Local,
			}

			cmd := &cobra.Command{
				PreRunE: runner.preRunE,
				RunE:    runner.runE,
			}
			cmd.SetIn(strings.NewReader(tc.input))
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetArgs([]string{})

			err := cmd.Execute()

			if tc.expectedErrMsg != "" {
				if !assert.Error(t, err) {
					t.FailNow()
				}
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, strings.TrimSpace(buf.String()), strings.TrimSpace(tc.expectedOutput))
		})
	}

	for tn, tc := range jsonTestCases {
		t.Run(tn, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace("namespace")
			defer tf.Cleanup()

			loader := manifestreader.NewFakeLoader(tf, tc.inventory)
			runner := &Runner{
				factory:    tf,
				invFactory: inventory.FakeClientFactory(tc.inventory),
				loader:     NewInventoryLoader(loader),
				PollerFactoryFunc: func(c cmdutil.Factory) (poller.Poller, error) {
					return &fakePoller{tc.events}, nil
				},

				pollUntil: tc.pollUntil,
				output:    tc.printer,
				timeout:   tc.timeout,
				invType:   Local,
			}

			cmd := &cobra.Command{
				RunE: runner.runE,
			}
			cmd.SetIn(strings.NewReader(tc.input))
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetArgs([]string{})

			err := cmd.Execute()
			if tc.expectedErrMsg != "" {
				if !assert.Error(t, err) {
					t.FailNow()
				}
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			assert.NoError(t, err)
			actual := strings.Split(buf.String(), "\n")
			assertOutput(t, tc.expectedOutput, actual)
		})
	}
}

// nolint:unparam
func assertOutput(t *testing.T, expectedOutput []map[string]interface{}, actual []string) bool {
	for i, expectedMap := range expectedOutput {
		if len(expectedMap) == 0 {
			return assert.Empty(t, actual[i])
		}

		var m map[string]interface{}
		err := json.Unmarshal([]byte(actual[i]), &m)
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
		if !assert.Equal(t, expectedMap, m) {
			return false
		}
	}
	return true
}

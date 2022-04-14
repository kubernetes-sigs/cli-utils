// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	printcommon "sigs.k8s.io/cli-utils/pkg/print/common"
	"sigs.k8s.io/cli-utils/pkg/print/stats"
	"sigs.k8s.io/cli-utils/pkg/printers/printer"
)

type PrinterFactoryFunc func() printer.Printer

func PrintResultErrorTest(t *testing.T, f PrinterFactoryFunc) {
	deploymentIdentifier := object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Name:      "foo",
		Namespace: "bar",
	}

	testCases := map[string]struct {
		events      []event.Event
		expectedErr error
	}{
		"successful apply, prune and reconcile": {
			events: []event.Event{
				{
					Type: event.InitType,
					InitEvent: event.InitEvent{
						ActionGroups: event.ActionGroupList{
							{
								Name:   "apply-1",
								Action: event.ApplyAction,
								Identifiers: []object.ObjMetadata{
									deploymentIdentifier,
								},
							},
							{
								Name:   "wait-1",
								Action: event.WaitAction,
								Identifiers: []object.ObjMetadata{
									deploymentIdentifier,
								},
							},
						},
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "apply-1",
						Action:    event.ApplyAction,
						Status:    event.Started,
					},
				},
				{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						GroupName:  "apply-1",
						Status:     event.ApplySuccessful,
						Identifier: deploymentIdentifier,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "apply-1",
						Action:    event.ApplyAction,
						Status:    event.Finished,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "wait-1",
						Action:    event.WaitAction,
						Status:    event.Started,
					},
				},
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  "wait-1",
						Status:     event.ReconcileSuccessful,
						Identifier: deploymentIdentifier,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "wait-1",
						Action:    event.WaitAction,
						Status:    event.Finished,
					},
				},
			},
			expectedErr: nil,
		},
		"successful apply, failed reconcile": {
			events: []event.Event{
				{
					Type: event.InitType,
					InitEvent: event.InitEvent{
						ActionGroups: event.ActionGroupList{
							{
								Name:   "apply-1",
								Action: event.ApplyAction,
								Identifiers: []object.ObjMetadata{
									deploymentIdentifier,
								},
							},
							{
								Name:   "wait-1",
								Action: event.WaitAction,
								Identifiers: []object.ObjMetadata{
									deploymentIdentifier,
								},
							},
						},
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "apply-1",
						Action:    event.ApplyAction,
						Status:    event.Started,
					},
				},
				{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						GroupName:  "apply-1",
						Status:     event.ApplySuccessful,
						Identifier: deploymentIdentifier,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "apply-1",
						Action:    event.ApplyAction,
						Status:    event.Finished,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "wait-1",
						Action:    event.WaitAction,
						Status:    event.Started,
					},
				},
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  "wait-1",
						Status:     event.ReconcileFailed,
						Identifier: deploymentIdentifier,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "wait-1",
						Action:    event.WaitAction,
						Status:    event.Finished,
					},
				},
			},
			expectedErr: &printcommon.ResultError{
				Stats: stats.Stats{
					ApplyStats: stats.ApplyStats{
						Successful: 1,
					},
					WaitStats: stats.WaitStats{
						Failed: 1,
					},
				},
			},
		},
		"failed apply": {
			events: []event.Event{
				{
					Type: event.InitType,
					InitEvent: event.InitEvent{
						ActionGroups: event.ActionGroupList{
							{
								Name:   "apply-1",
								Action: event.ApplyAction,
								Identifiers: []object.ObjMetadata{
									deploymentIdentifier,
								},
							},
							{
								Name:   "wait-1",
								Action: event.WaitAction,
								Identifiers: []object.ObjMetadata{
									deploymentIdentifier,
								},
							},
						},
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "apply-1",
						Action:    event.ApplyAction,
						Status:    event.Started,
					},
				},
				{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						GroupName:  "apply-1",
						Status:     event.ApplyFailed,
						Identifier: deploymentIdentifier,
						Error:      fmt.Errorf("apply failed"),
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "apply-1",
						Action:    event.ApplyAction,
						Status:    event.Finished,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "wait-1",
						Action:    event.WaitAction,
						Status:    event.Started,
					},
				},
				{
					Type: event.WaitType,
					WaitEvent: event.WaitEvent{
						GroupName:  "wait-1",
						Status:     event.ReconcileSkipped,
						Identifier: deploymentIdentifier,
					},
				},
				{
					Type: event.ActionGroupType,
					ActionGroupEvent: event.ActionGroupEvent{
						GroupName: "wait-1",
						Action:    event.WaitAction,
						Status:    event.Finished,
					},
				},
			},
			expectedErr: &printcommon.ResultError{
				Stats: stats.Stats{
					ApplyStats: stats.ApplyStats{
						Failed: 1,
					},
					WaitStats: stats.WaitStats{
						Skipped: 1,
					},
				},
			},
		},
	}

	for tn := range testCases {
		tc := testCases[tn]
		t.Run(tn, func(t *testing.T) {
			p := f()

			eventChannel := make(chan event.Event)

			var wg sync.WaitGroup
			var err error

			wg.Add(1)
			go func() {
				err = p.Print(eventChannel, common.DryRunNone, false)
				wg.Done()
			}()

			for i := range tc.events {
				e := tc.events[i]
				eventChannel <- e
			}
			close(eventChannel)

			wg.Wait()

			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

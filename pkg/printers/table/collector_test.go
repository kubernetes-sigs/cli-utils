// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package table

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	pe "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/graph"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
)

var (
	depID = object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Name:      "foo",
		Namespace: "default",
	}
	depID2 = object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Name:      "bar",
		Namespace: "default",
	}
	customID = object.ObjMetadata{
		GroupKind: schema.GroupKind{
			Group: "custom.io",
			Kind:  "Custom",
		},
		Name: "Custom",
	}
)

const testMessage = "test message for ResourceStatus"

func TestResourceStateCollector_New(t *testing.T) {
	testCases := map[string]struct {
		resourceGroups []event.ActionGroup
		resourceInfos  map[object.ObjMetadata]*resourceInfo
	}{
		"no resources": {
			resourceGroups: []event.ActionGroup{},
			resourceInfos:  map[object.ObjMetadata]*resourceInfo{},
		},
		"several resources for apply": {
			resourceGroups: []event.ActionGroup{
				{
					Action: event.ApplyAction,
					Identifiers: object.ObjMetadataSet{
						depID, customID,
					},
				},
			},
			resourceInfos: map[object.ObjMetadata]*resourceInfo{
				depID: {
					ResourceAction: event.ApplyAction,
				},
				customID: {
					ResourceAction: event.ApplyAction,
				},
			},
		},
		"several resources for prune": {
			resourceGroups: []event.ActionGroup{
				{
					Action: event.ApplyAction,
					Identifiers: object.ObjMetadataSet{
						customID,
					},
				},
				{
					Action: event.PruneAction,
					Identifiers: object.ObjMetadataSet{
						depID,
					},
				},
			},
			resourceInfos: map[object.ObjMetadata]*resourceInfo{
				depID: {
					ResourceAction: event.PruneAction,
				},
				customID: {
					ResourceAction: event.ApplyAction,
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			rsc := newResourceStateCollector(tc.resourceGroups)

			assert.Equal(t, len(tc.resourceInfos), len(rsc.resourceInfos))
			for expID, expRi := range tc.resourceInfos {
				actRi, found := rsc.resourceInfos[expID]
				if !found {
					t.Errorf("expected to find id %v, but didn't", expID)
				}
				assert.Equal(t, expRi.ResourceAction, actRi.ResourceAction)
			}
		})
	}
}

func TestResourceStateCollector_ProcessStatusEvent(t *testing.T) {
	testCases := map[string]struct {
		resourceGroups []event.ActionGroup
		statusEvent    event.StatusEvent
	}{
		"nil StatusEvent.Resource does not crash": {
			resourceGroups: []event.ActionGroup{},
			statusEvent: event.StatusEvent{
				Resource: nil,
			},
		},
		"unfound Resource identifier does not crash": {
			resourceGroups: []event.ActionGroup{
				{
					Action:      event.ApplyAction,
					Identifiers: object.ObjMetadataSet{depID},
				},
			},
			statusEvent: event.StatusEvent{
				PollResourceInfo: &pe.ResourceStatus{
					Identifier: customID, // Does not match identifier in resourceGroups
				},
			},
		},
		"basic status event for applying two resources updates resourceStatus": {
			resourceGroups: []event.ActionGroup{
				{
					Action: event.ApplyAction,
					Identifiers: object.ObjMetadataSet{
						depID, customID,
					},
				},
			},
			statusEvent: event.StatusEvent{
				PollResourceInfo: &pe.ResourceStatus{
					Identifier: depID,
					Message:    testMessage,
				},
			},
		},
		"several resources for prune": {
			resourceGroups: []event.ActionGroup{
				{
					Action: event.ApplyAction,
					Identifiers: object.ObjMetadataSet{
						customID,
					},
				},
				{
					Action: event.PruneAction,
					Identifiers: object.ObjMetadataSet{
						depID,
					},
				},
			},
			statusEvent: event.StatusEvent{
				PollResourceInfo: &pe.ResourceStatus{
					Identifier: depID,
					Message:    testMessage,
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			rsc := newResourceStateCollector(tc.resourceGroups)
			rsc.processStatusEvent(tc.statusEvent)
			id, found := getID(tc.statusEvent)
			if found {
				resourceInfo, found := rsc.resourceInfos[id]
				if found {
					// Validate the ResourceStatus was set from StatusEvent
					if resourceInfo.resourceStatus != tc.statusEvent.PollResourceInfo {
						t.Errorf("status event not processed for %s", id)
					}
				}
			}
		})
	}
}

func TestResourceStateCollector_ProcessValidationEvent(t *testing.T) {
	testCases := map[string]struct {
		resourceGroups []event.ActionGroup
		event          event.ValidationEvent
		expectedError  error
	}{
		"zero objects, return error": {
			event: event.ValidationEvent{
				Identifiers: object.ObjMetadataSet{},
				Error:       errors.New("unexpected"),
			},
			expectedError: errors.New("invalid validation event: no identifiers: unexpected"),
		},
		"one object, missing namespace": {
			resourceGroups: []event.ActionGroup{
				{
					Action:      event.ApplyAction,
					Identifiers: object.ObjMetadataSet{depID},
				},
			},
			event: event.ValidationEvent{
				Identifiers: object.ObjMetadataSet{depID},
				Error: validation.NewError(
					field.Required(field.NewPath("metadata", "namespace"), "namespace is required"),
					depID,
				),
			},
		},
		"two objects, cyclic dependency": {
			event: event.ValidationEvent{
				Identifiers: object.ObjMetadataSet{depID, depID2},
				Error: validation.NewError(
					graph.CyclicDependencyError{
						Edges: []graph.Edge{
							{
								From: depID,
								To:   depID2,
							},
							{
								From: depID2,
								To:   depID,
							},
						},
					},
					depID,
					depID2,
				),
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			rsc := newResourceStateCollector(tc.resourceGroups)
			err := rsc.processValidationEvent(tc.event)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			for _, id := range tc.event.Identifiers {
				resourceInfo, found := rsc.resourceInfos[id]
				if found {
					assert.Equal(t, &pe.ResourceStatus{
						Identifier: id,
						Status:     InvalidStatus,
						Message:    tc.event.Error.Error(),
					}, resourceInfo.resourceStatus)
				}
			}
		})
	}
}

func getID(e event.StatusEvent) (object.ObjMetadata, bool) {
	if e.Resource == nil {
		return object.ObjMetadata{}, false
	}
	return e.Identifier, true
}

// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestEventFactory(t *testing.T) {
	tests := map[string]struct {
		destroy      bool
		obj          *unstructured.Unstructured
		skippedErr   error
		failedErr    error
		expectedType event.Type
	}{
		"prune events": {
			destroy:      false,
			obj:          pod,
			skippedErr:   fmt.Errorf("fake reason"),
			expectedType: event.PruneType,
		},
		"delete events": {
			destroy:      true,
			obj:          pdb,
			skippedErr:   fmt.Errorf("fake reason"),
			expectedType: event.DeleteType,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			id := object.UnstructuredToObjMetadata(tc.obj)
			eventFactory := CreateEventFactory(tc.destroy, "task-0")
			// Validate the "success" event"
			actualEvent := eventFactory.CreateSuccessEvent(tc.obj)
			if tc.expectedType != actualEvent.Type {
				t.Errorf("success event expected type (%s), got (%s)",
					tc.expectedType, actualEvent.Type)
			}
			var actualObj *unstructured.Unstructured
			var err error
			if tc.expectedType == event.PruneType {
				if event.PruneSuccessful != actualEvent.PruneEvent.Status {
					t.Errorf("success event expected status (PruneSuccessful), got (%s)",
						actualEvent.PruneEvent.Status)
				}
				actualObj = actualEvent.PruneEvent.Object
				err = actualEvent.PruneEvent.Error
			} else {
				if event.DeleteSuccessful != actualEvent.DeleteEvent.Status {
					t.Errorf("success event expected status (DeleteSuccessful), got (%s)",
						actualEvent.DeleteEvent.Status)
				}
				actualObj = actualEvent.DeleteEvent.Object
				err = actualEvent.DeleteEvent.Error
			}
			if tc.obj != actualObj {
				t.Errorf("expected event object (%v), got (%v)", tc.obj, actualObj)
			}
			if err != nil {
				t.Errorf("success event expected nil error, got (%s)", err)
			}
			// Validate the "skipped" event"
			actualEvent = eventFactory.CreateSkippedEvent(tc.obj, tc.skippedErr)
			if tc.expectedType != actualEvent.Type {
				t.Errorf("skipped event expected type (%s), got (%s)",
					tc.expectedType, actualEvent.Type)
			}
			if tc.expectedType == event.PruneType {
				if event.PruneSkipped != actualEvent.PruneEvent.Status {
					t.Errorf("skipped event expected status (PruneSkipped), got (%s)",
						actualEvent.PruneEvent.Status)
				}
				actualObj = actualEvent.PruneEvent.Object
				err = actualEvent.PruneEvent.Error
			} else {
				if event.DeleteSkipped != actualEvent.DeleteEvent.Status {
					t.Errorf("skipped event expected status (DeleteSkipped), got (%s)",
						actualEvent.DeleteEvent.Status)
				}
				actualObj = actualEvent.DeleteEvent.Object
				err = actualEvent.DeleteEvent.Error
			}
			if tc.obj != actualObj {
				t.Errorf("expected event object (%v), got (%v)", tc.obj, actualObj)
			}
			if tc.skippedErr != err {
				t.Errorf("skipped event expected error (%s), got (%s)", tc.skippedErr, err)
			}
			// Validate the "failed" event"
			actualEvent = eventFactory.CreateFailedEvent(id, tc.failedErr)
			if tc.expectedType != actualEvent.Type {
				t.Errorf("failed event expected type (%s), got (%s)",
					tc.expectedType, actualEvent.Type)
			}
			if tc.expectedType != actualEvent.Type {
				t.Errorf("failed event expected type (%s), got (%s)",
					tc.expectedType, actualEvent.Type)
			}
			if tc.expectedType == event.PruneType {
				err = actualEvent.PruneEvent.Error
			} else {
				err = actualEvent.DeleteEvent.Error
			}
			if tc.failedErr != err {
				t.Errorf("failed event expected error (%s), got (%s)", tc.failedErr, err)
			}
		})
	}
}

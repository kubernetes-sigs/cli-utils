// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

var resourceIdentifiers = map[string]wait.ResourceIdentifier{
	"deployment": {
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "Deployment",
		},
		Name:      "Foo",
		Namespace: "default",
	},
	"statefulset": {
		GroupKind: schema.GroupKind{
			Group: "apps",
			Kind:  "StatefulSet",
		},
		Name:      "Bar",
		Namespace: "default",
	},
	"service": {
		GroupKind: schema.GroupKind{
			Group: "",
			Kind:  "Service",
		},
		Name:      "Service",
		Namespace: "default",
	},
}

func TestAggregator(t *testing.T) {
	testCases := map[string]struct {
		identifiers      []wait.ResourceIdentifier
		resourceStatuses []event.ResourceStatus
		aggregateStatus  status.Status
	}{
		"no identifiers": {
			identifiers:      []wait.ResourceIdentifier{},
			resourceStatuses: []event.ResourceStatus{},
			aggregateStatus:  status.CurrentStatus,
		},
		"single identifier with multiple resourceStatuses": {
			identifiers: []wait.ResourceIdentifier{resourceIdentifiers["deployment"]},
			resourceStatuses: []event.ResourceStatus{
				{
					Identifier: resourceIdentifiers["deployment"],
					Status:     status.UnknownStatus,
				},
				{
					Identifier: resourceIdentifiers["deployment"],
					Status:     status.InProgressStatus,
				},
			},
			aggregateStatus: status.InProgressStatus,
		},
		"multiple resources with one unknown status": {
			identifiers: []wait.ResourceIdentifier{
				resourceIdentifiers["deployment"],
				resourceIdentifiers["statefulset"],
			},
			resourceStatuses: []event.ResourceStatus{
				{
					Identifier: resourceIdentifiers["deployment"],
					Status:     status.UnknownStatus,
				},
				{
					Identifier: resourceIdentifiers["statefulset"],
					Status:     status.InProgressStatus,
				},
			},
			aggregateStatus: status.UnknownStatus,
		},
		"multiple resources with one failed": {
			identifiers: []wait.ResourceIdentifier{
				resourceIdentifiers["deployment"],
				resourceIdentifiers["statefulset"],
				resourceIdentifiers["service"],
			},
			resourceStatuses: []event.ResourceStatus{
				{
					Identifier: resourceIdentifiers["deployment"],
					Status:     status.NotFoundStatus,
				},
				{
					Identifier: resourceIdentifiers["statefulset"],
					Status:     status.CurrentStatus,
				},
				{
					Identifier: resourceIdentifiers["service"],
					Status:     status.FailedStatus,
				},
			},
			aggregateStatus: status.FailedStatus,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			aggregator := newGenericAggregator(tc.identifiers, status.CurrentStatus)

			for _, rs := range tc.resourceStatuses {
				resourceStatus := rs
				aggregator.ResourceStatus(&resourceStatus)
			}

			aggStatus := aggregator.AggregateStatus()

			assert.Equal(t, tc.aggregateStatus, aggStatus)
		})
	}
}

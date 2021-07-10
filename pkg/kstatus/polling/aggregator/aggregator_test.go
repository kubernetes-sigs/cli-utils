// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var resourceIdentifiers = map[string]object.ObjMetadata{
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
		desiredStatus    status.Status
		resourceStatuses []*event.ResourceStatus
		aggregateStatus  status.Status
	}{
		"no identifiers": {
			desiredStatus:    status.CurrentStatus,
			resourceStatuses: []*event.ResourceStatus{},
			aggregateStatus:  status.CurrentStatus,
		},
		"single resource": {
			desiredStatus: status.CurrentStatus,
			resourceStatuses: []*event.ResourceStatus{
				{
					Identifier: resourceIdentifiers["deployment"],
					Status:     status.CurrentStatus,
				},
			},
			aggregateStatus: status.CurrentStatus,
		},
		"multiple resources with one unknown status": {
			desiredStatus: status.CurrentStatus,
			resourceStatuses: []*event.ResourceStatus{
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
			desiredStatus: status.CurrentStatus,
			resourceStatuses: []*event.ResourceStatus{
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
			aggStatus := AggregateStatus(tc.resourceStatuses, tc.desiredStatus)

			assert.Equal(t, tc.aggregateStatus, aggStatus)
		})
	}
}

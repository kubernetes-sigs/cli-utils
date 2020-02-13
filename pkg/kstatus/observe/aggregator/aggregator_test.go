package aggregator

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
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
		identifiers     []wait.ResourceIdentifier
		observations    []event.ObservedResource
		aggregateStatus status.Status
	}{
		"no identifiers": {
			identifiers:     []wait.ResourceIdentifier{},
			observations:    []event.ObservedResource{},
			aggregateStatus: status.CurrentStatus,
		},
		"single identifier with multiple observations": {
			identifiers: []wait.ResourceIdentifier{resourceIdentifiers["deployment"]},
			observations: []event.ObservedResource{
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
			observations: []event.ObservedResource{
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
		"multiple resources with all current or not found": {
			identifiers: []wait.ResourceIdentifier{
				resourceIdentifiers["deployment"],
				resourceIdentifiers["statefulset"],
			},
			observations: []event.ObservedResource{
				{
					Identifier: resourceIdentifiers["deployment"],
					Status:     status.NotFoundStatus,
				},
				{
					Identifier: resourceIdentifiers["statefulset"],
					Status:     status.CurrentStatus,
				},
			},
			aggregateStatus: status.CurrentStatus,
		},
		"multiple resources with one failed": {
			identifiers: []wait.ResourceIdentifier{
				resourceIdentifiers["deployment"],
				resourceIdentifiers["statefulset"],
				resourceIdentifiers["service"],
			},
			observations: []event.ObservedResource{
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
			aggregator := NewAllCurrentOrNotFoundStatusAggregator(tc.identifiers)

			for _, o := range tc.observations {
				observation := o
				aggregator.ResourceObserved(&observation)
			}

			aggStatus := aggregator.AggregateStatus()

			assert.Equal(t, tc.aggregateStatus, aggStatus)
		})
	}
}

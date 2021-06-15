// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestCurrentUIDFilter(t *testing.T) {
	tests := map[string]struct {
		filterUIDs sets.String
		objUID     string
		filtered   bool
	}{
		"Empty filter UIDs, object is not filtered": {
			filterUIDs: sets.NewString(),
			objUID:     "bar",
			filtered:   false,
		},
		"Empty object UID, object is not filtered": {
			filterUIDs: sets.NewString("foo"),
			objUID:     "",
			filtered:   false,
		},
		"Object UID not in filter UID set, object is not filtered": {
			filterUIDs: sets.NewString("foo", "baz"),
			objUID:     "bar",
			filtered:   false,
		},
		"Object UID is in filter UID set, object is filtered": {
			filterUIDs: sets.NewString("foo"),
			objUID:     "foo",
			filtered:   true,
		},
		"Object UID is among several filter UIDs, object is filtered": {
			filterUIDs: sets.NewString("foo", "bar", "baz"),
			objUID:     "foo",
			filtered:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := CurrentUIDFilter{
				CurrentUIDs: tc.filterUIDs,
			}
			obj := defaultObj.DeepCopy()
			obj.SetUID(types.UID(tc.objUID))
			actual, err := filter.Filter(obj)
			if err != nil {
				t.Fatalf("CurrentUIDFilter unexpected error (%s)", err)
			}
			if tc.filtered != actual {
				t.Errorf("CurrentUIDFilter expected filter (%t), got (%t)", tc.filtered, actual)
			}
		})
	}
}

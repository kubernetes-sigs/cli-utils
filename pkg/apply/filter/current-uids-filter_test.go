// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestCurrentUIDFilter(t *testing.T) {
	tests := map[string]struct {
		filterUIDs    sets.Set[types.UID] // nolint:staticcheck
		objUID        types.UID
		expectedError error
	}{
		"Empty filter UIDs, object is not filtered": {
			filterUIDs: sets.New[types.UID](),
			objUID:     "bar",
		},
		"Empty object UID, object is not filtered": {
			filterUIDs: sets.New[types.UID]("foo"),
			objUID:     "",
		},
		"Object UID not in filter UID set, object is not filtered": {
			filterUIDs: sets.New[types.UID]("foo", "baz"),
			objUID:     "bar",
		},
		"Object UID is in filter UID set, object is filtered": {
			filterUIDs:    sets.New[types.UID]("foo"),
			objUID:        "foo",
			expectedError: &ApplyPreventedDeletionError{UID: "foo"},
		},
		"Object UID is among several filter UIDs, object is filtered": {
			filterUIDs:    sets.New[types.UID]("foo", "bar", "baz"),
			objUID:        "foo",
			expectedError: &ApplyPreventedDeletionError{UID: "foo"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := CurrentUIDFilter{
				CurrentUIDs: tc.filterUIDs,
			}
			obj := defaultObj.DeepCopy()
			obj.SetUID(tc.objUID)
			err := filter.Filter(t.Context(), obj)
			testutil.AssertEqual(t, tc.expectedError, err)
		})
	}
}

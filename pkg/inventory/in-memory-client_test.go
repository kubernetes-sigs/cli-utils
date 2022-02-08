// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
)

func TestInMemoryClient(t *testing.T) {
	ref1 := actuation.ObjectReference{Kind: "Pod", Name: "pod-1"}

	tests := map[string]struct {
		input            *actuation.Inventory
		expectedStoreErr error
		expected         *actuation.Inventory
		expectedLoadErr  error
	}{
		"empty": {
			input:    &actuation.Inventory{},
			expected: &actuation.Inventory{},
		},
		"nil": {
			input:            nil,
			expectedStoreErr: errors.New("inventory must not be nil"),
		},
		"one spec object": {
			input: &actuation.Inventory{
				Spec: actuation.InventorySpec{
					Objects: []actuation.ObjectReference{ref1},
				},
			},
			expected: &actuation.Inventory{Spec: actuation.InventorySpec{Objects: []actuation.ObjectReference{ref1}}},
		},
		"one spec object, one status object": {
			input: &actuation.Inventory{
				Spec: actuation.InventorySpec{
					Objects: []actuation.ObjectReference{ref1},
				},
				Status: actuation.InventoryStatus{
					Objects: []actuation.ObjectStatus{
						{
							ObjectReference: ref1,
							Strategy:        actuation.ActuationStrategyApply,
							Actuation:       actuation.ActuationPending,
							Reconcile:       actuation.ReconcilePending,
						},
					},
				},
			},
			expected: &actuation.Inventory{
				Spec: actuation.InventorySpec{
					Objects: []actuation.ObjectReference{ref1},
				},
				Status: actuation.InventoryStatus{
					Objects: []actuation.ObjectStatus{
						{
							ObjectReference: ref1,
							Strategy:        actuation.ActuationStrategyApply,
							Actuation:       actuation.ActuationPending,
							Reconcile:       actuation.ReconcilePending,
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := &InMemoryClient{}

			err := client.Store(tc.input)
			if tc.expectedStoreErr != nil {
				assert.EqualError(t, err, tc.expectedStoreErr.Error())
				return
			}
			assert.NoError(t, err)

			invInfo := InventoryInfoFromObject(tc.input)
			inv, err := client.Load(invInfo)
			assert.Equal(t, tc.expected, inv)
		})
	}
}

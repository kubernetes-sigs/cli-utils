// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

// AddApplyTimeMutation returns a testutil.Mutator which adds the passed objects
// as an apply-time-mutation annotation to the object which is mutated. Multiple
// objects passed in means multiple substitutions in the annotation yaml list.
func AddApplyTimeMutation(t *testing.T, mutation *mutation.ApplyTimeMutation) testutil.Mutator {
	return applyTimeMutationMutator{
		t:        t,
		mutation: mutation,
	}
}

// applyTimeMutationMutator encapsulates fields for adding apply-time-mutation
// annotation to a test object. Implements the Mutator interface.
type applyTimeMutationMutator struct {
	t        *testing.T
	mutation *mutation.ApplyTimeMutation
}

// Mutate for applyTimeMutationMutator sets the apply-time-mutation annotation
// on the passed object.
func (a applyTimeMutationMutator) Mutate(u *unstructured.Unstructured) {
	err := mutation.WriteAnnotation(u, *a.mutation)
	if !assert.NoError(a.t, err) {
		a.t.FailNow()
	}
}

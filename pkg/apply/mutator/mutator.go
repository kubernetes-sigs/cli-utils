// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package mutator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Interface decouples apply-time-mutation
// from the concrete structs used for applying.
type Interface interface {
	// Name returns a filter name (usually for logging).
	Name() string
	// Mutate returns true if the object was mutated.
	// This allows the mutator to decide if mutation is needed.
	// If mutated, a reason string is returned.
	// If an error happens during mutation, it is returned.
	Mutate(ctx context.Context, obj *unstructured.Unstructured) (bool, string, error)
}

// Mutate the object with the supplied mutators, returning the first error.
func Mutate(ctx context.Context, obj *unstructured.Unstructured, mutators []Interface) error {
	id := object.UnstructuredToObjMetaOrDie(obj)
	for _, mutator := range mutators {
		klog.V(6).Infof("performing mutation (mutator: %q, resource: %q): %s", mutator.Name(), id)
		mutated, reason, err := mutator.Mutate(ctx, obj)
		if err != nil {
			return fmt.Errorf("failed to mutate %q with %q: %w", id, mutator.Name(), err)
		}
		if mutated {
			klog.V(4).Infof("resource mutated (mutator: %q, resource: %q, reason: %q)", mutator.Name(), id, reason)
		}
	}
	return nil
}

// MutateAll loops through the objects and runs Mutate on each, returning the
// first error.
func MutateAll(ctx context.Context, objs []*unstructured.Unstructured, mutators []Interface) error {
	for _, obj := range objs {
		err := Mutate(ctx, obj, mutators)
		if err != nil {
			return err
		}
	}
	return nil
}

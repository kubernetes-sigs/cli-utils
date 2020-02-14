// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package observer

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

// ComputeStatusFunc is the function that the observer will delegate to for
// computing status of the resource. This is set to use the status library
// by default and changing it is probably only useful for testing.
type ComputeStatusFunc func(u *unstructured.Unstructured) (*status.Result, error)

// ResourceObserver is the main interface for observers. In this context,
// an observer is an object that can fetch a resource of a specific
// GroupKind from the cluster and compute its status. For resources that
// can own generated resources, the observer might also have knowledge about
// how to identify these generated resources and how to compute status for
// these generated resources.
type ResourceObserver interface {
	// Observe will fetch the resource identified by the given identifier
	// from the cluster and return an ObservedResource that will contain
	// information about the latest state of the resource, its computed status
	// and information about any generated resources.
	Observe(ctx context.Context, resource wait.ResourceIdentifier) *event.ObservedResource

	// ObserveObject is similar to Observe, but instead of looking up the
	// resource based on an identifier, it will use the passed in resource.
	ObserveObject(ctx context.Context, object *unstructured.Unstructured) *event.ObservedResource

	// SetComputeStatusFunc can be used to set the function invoked by
	// the observer to compute the status of a resource. By default this
	// is set to use the status library. This is only used for testing.
	SetComputeStatusFunc(statusFunc ComputeStatusFunc)
}

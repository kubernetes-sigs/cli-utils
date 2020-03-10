// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// StatusReader is the main interface for computing status for resources. In this context,
// a status reader is an object that can fetch a resource of a specific
// GroupKind from the cluster and compute its status. For resources that
// can own generated resources, the engine might also have knowledge about
// how to identify these generated resources and how to compute status for
// these generated resources.
type StatusReader interface {
	// ReadStatus will fetch the resource identified by the given identifier
	// from the cluster and return an ResourceStatus that will contain
	// information about the latest state of the resource, its computed status
	// and information about any generated resources.
	ReadStatus(ctx context.Context, resource object.ObjMetadata) *event.ResourceStatus

	// ReadStatusForObject is similar to ReadStatus, but instead of looking up the
	// resource based on an identifier, it will use the passed-in resource.
	ReadStatusForObject(ctx context.Context, object *unstructured.Unstructured) *event.ResourceStatus
}

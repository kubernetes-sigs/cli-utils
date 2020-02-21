// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package observers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/observer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

func NewGenericObserver(reader observer.ClusterReader, mapper meta.RESTMapper) observer.ResourceObserver {
	return &genericObserver{
		BaseObserver: BaseObserver{
			Reader:            reader,
			Mapper:            mapper,
			computeStatusFunc: status.Compute,
		},
	}
}

// genericObserver is an observer that will be used for any resource that
// doesn't have a specific observer. It will just delegate computation of
// status to the status library.
// This should work pretty well for resources that doesn't have any
// generated resources and where status can be computed only based on the
// resource itself.
type genericObserver struct {
	BaseObserver
}

var _ observer.ResourceObserver = &genericObserver{}

func (d *genericObserver) Observe(ctx context.Context, identifier wait.ResourceIdentifier) *event.ObservedResource {
	u, err := d.LookupResource(ctx, identifier)
	if err != nil {
		return d.handleObservedResourceError(identifier, err)
	}
	return d.ObserveObject(ctx, u)
}

func (d *genericObserver) ObserveObject(_ context.Context, resource *unstructured.Unstructured) *event.ObservedResource {
	identifier := toIdentifier(resource)

	res, err := d.computeStatusFunc(resource)
	if err != nil {
		return &event.ObservedResource{
			Identifier: identifier,
			Status:     status.UnknownStatus,
			Error:      err,
		}
	}

	return &event.ObservedResource{
		Identifier: identifier,
		Status:     res.Status,
		Resource:   resource,
		Message:    res.Message,
	}
}

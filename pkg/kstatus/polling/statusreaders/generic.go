// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func NewGenericStatusReader(reader engine.ClusterReader, mapper meta.RESTMapper) engine.StatusReader {
	return &genericStatusReader{
		BaseStatusReader: BaseStatusReader{
			Reader:            reader,
			Mapper:            mapper,
			computeStatusFunc: status.Compute,
		},
	}
}

// genericStatusReader is an engine that will be used for any resource that
// doesn't have a specific engine. It will just delegate computation of
// status to the status library.
// This should work pretty well for resources that doesn't have any
// generated resources and where status can be computed only based on the
// resource itself.
type genericStatusReader struct {
	BaseStatusReader
}

var _ engine.StatusReader = &genericStatusReader{}

func (d *genericStatusReader) ReadStatus(ctx context.Context, identifier object.ObjMetadata) *event.ResourceStatus {
	u, err := d.LookupResource(ctx, identifier)
	if err != nil {
		return d.handleResourceStatusError(identifier, err)
	}
	return d.ReadStatusForObject(ctx, u)
}

func (d *genericStatusReader) ReadStatusForObject(_ context.Context, resource *unstructured.Unstructured) *event.ResourceStatus {
	identifier := toIdentifier(resource)

	res, err := d.computeStatusFunc(resource)
	if err != nil {
		return &event.ResourceStatus{
			Identifier: identifier,
			Status:     status.UnknownStatus,
			Error:      err,
		}
	}

	return &event.ResourceStatus{
		Identifier: identifier,
		Status:     res.Status,
		Resource:   resource,
		Message:    res.Message,
	}
}

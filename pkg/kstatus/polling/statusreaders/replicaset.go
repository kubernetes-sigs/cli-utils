// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func NewReplicaSetStatusReader(reader engine.ClusterReader, mapper meta.RESTMapper, podStatusReader engine.StatusReader) engine.StatusReader {
	return &replicaSetStatusReader{
		BaseStatusReader: BaseStatusReader{
			Reader:            reader,
			Mapper:            mapper,
			computeStatusFunc: status.Compute,
		},
		PodStatusReader: podStatusReader,
	}
}

// replicaSetStatusReader is an engine that can fetch ReplicaSet resources
// from the cluster, knows how to find any Pods belonging to the ReplicaSet,
// and compute status for the ReplicaSet.
type replicaSetStatusReader struct {
	BaseStatusReader

	PodStatusReader engine.StatusReader
}

var _ engine.StatusReader = &replicaSetStatusReader{}

func (r *replicaSetStatusReader) ReadStatus(ctx context.Context, identifier object.ObjMetadata) *event.ResourceStatus {
	rs, err := r.LookupResource(ctx, identifier)
	if err != nil {
		return r.handleResourceStatusError(identifier, err)
	}
	return r.ReadStatusForObject(ctx, rs)
}

func (r *replicaSetStatusReader) ReadStatusForObject(ctx context.Context, rs *unstructured.Unstructured) *event.ResourceStatus {
	identifier := toIdentifier(rs)

	podResourceStatuses, err := r.StatusForGeneratedResources(ctx, r.PodStatusReader, rs,
		v1.SchemeGroupVersion.WithKind("Pod").GroupKind(), "spec", "selector")
	if err != nil {
		return &event.ResourceStatus{
			Identifier: identifier,
			Status:     status.UnknownStatus,
			Resource:   rs,
			Error:      err,
		}
	}

	res, err := r.computeStatusFunc(rs)
	if err != nil {
		return &event.ResourceStatus{
			Identifier:         identifier,
			Status:             status.UnknownStatus,
			Error:              err,
			GeneratedResources: podResourceStatuses,
		}
	}

	return &event.ResourceStatus{
		Identifier:         identifier,
		Status:             res.Status,
		Resource:           rs,
		Message:            res.Message,
		GeneratedResources: podResourceStatuses,
	}
}

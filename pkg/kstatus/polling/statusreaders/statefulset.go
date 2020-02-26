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
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

func NewStatefulSetResourceReader(reader engine.ClusterReader, mapper meta.RESTMapper, podResourceReader engine.StatusReader) engine.StatusReader {
	return &statefulSetResourceReader{
		BaseStatusReader: BaseStatusReader{
			Reader:            reader,
			Mapper:            mapper,
			computeStatusFunc: status.Compute,
		},
		PodResourceReader: podResourceReader,
	}
}

// statefulSetResourceReader is an implementation of the ResourceReader interface
// that can fetch StatefulSet resources from the cluster, knows how to find any
// Pods belonging to the StatefulSet, and compute status for the StatefulSet.
type statefulSetResourceReader struct {
	BaseStatusReader

	PodResourceReader engine.StatusReader
}

var _ engine.StatusReader = &statefulSetResourceReader{}

func (s *statefulSetResourceReader) ReadStatus(ctx context.Context, identifier wait.ResourceIdentifier) *event.ResourceStatus {
	statefulSet, err := s.LookupResource(ctx, identifier)
	if err != nil {
		return s.handleResourceStatusError(identifier, err)
	}
	return s.ReadStatusForObject(ctx, statefulSet)
}

func (s *statefulSetResourceReader) ReadStatusForObject(ctx context.Context, statefulSet *unstructured.Unstructured) *event.ResourceStatus {
	identifier := toIdentifier(statefulSet)

	podResourceStatuses, err := s.StatusForGeneratedResources(ctx, s.PodResourceReader, statefulSet,
		v1.SchemeGroupVersion.WithKind("Pod").GroupKind(), "spec", "selector")
	if err != nil {
		return &event.ResourceStatus{
			Identifier: identifier,
			Status:     status.UnknownStatus,
			Resource:   statefulSet,
			Error:      err,
		}
	}

	res, err := s.computeStatusFunc(statefulSet)
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
		Resource:           statefulSet,
		Message:            res.Message,
		GeneratedResources: podResourceStatuses,
	}
}

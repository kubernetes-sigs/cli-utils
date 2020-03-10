// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
)

func NewStatefulSetResourceReader(reader engine.ClusterReader, mapper meta.RESTMapper, podResourceReader resourceTypeStatusReader) engine.StatusReader {
	return &baseStatusReader{
		reader: reader,
		mapper: mapper,
		resourceStatusReader: &statefulSetResourceReader{
			reader:            reader,
			mapper:            mapper,
			podResourceReader: podResourceReader,
		},
	}
}

// statefulSetResourceReader is an implementation of the ResourceReader interface
// that can fetch StatefulSet resources from the cluster, knows how to find any
// Pods belonging to the StatefulSet, and compute status for the StatefulSet.
type statefulSetResourceReader struct {
	reader engine.ClusterReader
	mapper meta.RESTMapper

	podResourceReader resourceTypeStatusReader
}

var _ resourceTypeStatusReader = &statefulSetResourceReader{}

func (s *statefulSetResourceReader) ReadStatusForObject(ctx context.Context, statefulSet *unstructured.Unstructured) *event.ResourceStatus {
	return newPodControllerStatusReader(s.reader, s.mapper, s.podResourceReader).readStatus(ctx, statefulSet)
}

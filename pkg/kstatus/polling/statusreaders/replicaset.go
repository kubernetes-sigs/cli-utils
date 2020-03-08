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

func NewReplicaSetStatusReader(reader engine.ClusterReader, mapper meta.RESTMapper, podStatusReader resourceTypeStatusReader) engine.StatusReader {
	return &baseStatusReader{
		reader: reader,
		mapper: mapper,
		resourceStatusReader: &replicaSetStatusReader{
			reader:          reader,
			mapper:          mapper,
			podStatusReader: podStatusReader,
		},
	}
}

// replicaSetStatusReader is an engine that can fetch ReplicaSet resources
// from the cluster, knows how to find any Pods belonging to the ReplicaSet,
// and compute status for the ReplicaSet.
type replicaSetStatusReader struct {
	reader engine.ClusterReader
	mapper meta.RESTMapper

	podStatusReader resourceTypeStatusReader
}

var _ resourceTypeStatusReader = &replicaSetStatusReader{}

func (r *replicaSetStatusReader) ReadStatusForObject(ctx context.Context, rs *unstructured.Unstructured) *event.ResourceStatus {
	return newPodControllerStatusReader(r.reader, r.mapper, r.podStatusReader).readStatus(ctx, rs)
}

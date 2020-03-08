// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

func NewDeploymentResourceReader(reader engine.ClusterReader, mapper meta.RESTMapper, rsStatusReader resourceTypeStatusReader) engine.StatusReader {
	return &baseStatusReader{
		reader: reader,
		mapper: mapper,
		resourceStatusReader: &deploymentResourceReader{
			reader:         reader,
			mapper:         mapper,
			rsStatusReader: rsStatusReader,
		},
	}
}

// deploymentResourceReader is a resourceTypeStatusReader that can fetch Deployment
// resources from the cluster, knows how to find any ReplicaSets belonging to the
// Deployment, and compute status for the deployment.
type deploymentResourceReader struct {
	reader engine.ClusterReader
	mapper meta.RESTMapper

	// rsStatusReader is the implementation of the resourceTypeStatusReader
	// the knows how to compute the status for ReplicaSets.
	rsStatusReader resourceTypeStatusReader
}

var _ resourceTypeStatusReader = &deploymentResourceReader{}

func (d *deploymentResourceReader) ReadStatusForObject(ctx context.Context, deployment *unstructured.Unstructured) *event.ResourceStatus {
	identifier := toIdentifier(deployment)

	replicaSetStatuses, err := statusForGeneratedResources(ctx, d.mapper, d.reader, d.rsStatusReader, deployment,
		appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind(), "spec", "selector")
	if err != nil {
		return &event.ResourceStatus{
			Identifier: identifier,
			Status:     status.UnknownStatus,
			Resource:   deployment,
			Error:      err,
		}
	}

	// Currently this engine just uses the status library for computing
	// status for the deployment. But we do have the status and state for all
	// ReplicaSets and Pods in the ObservedReplicaSets data structure, so the
	// rules can be improved to take advantage of this information.
	res, err := status.Compute(deployment)
	if err != nil {
		return &event.ResourceStatus{
			Identifier:         identifier,
			Status:             status.UnknownStatus,
			Error:              err,
			GeneratedResources: replicaSetStatuses,
		}
	}

	return &event.ResourceStatus{
		Identifier:         identifier,
		Status:             res.Status,
		Resource:           deployment,
		Message:            res.Message,
		GeneratedResources: replicaSetStatuses,
	}
}

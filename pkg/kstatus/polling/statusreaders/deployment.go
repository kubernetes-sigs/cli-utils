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
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

func NewDeploymentResourceReader(reader engine.ClusterReader, mapper meta.RESTMapper, rsStatusReader engine.StatusReader) engine.StatusReader {
	return &deploymentResourceReader{
		BaseStatusReader: BaseStatusReader{
			Reader:            reader,
			Mapper:            mapper,
			computeStatusFunc: status.Compute,
		},
		RsStatusReader: rsStatusReader,
	}
}

// deploymentResourceReader is an engine that can fetch Deployment resources
// from the cluster, knows how to find any ReplicaSets belonging to the
// Deployment, and compute status for the deployment.
type deploymentResourceReader struct {
	BaseStatusReader

	RsStatusReader engine.StatusReader
}

var _ engine.StatusReader = &deploymentResourceReader{}

func (d *deploymentResourceReader) ReadStatus(ctx context.Context, identifier wait.ResourceIdentifier) *event.ResourceStatus {
	deployment, err := d.LookupResource(ctx, identifier)
	if err != nil {
		return d.handleResourceStatusError(identifier, err)
	}
	return d.ReadStatusForObject(ctx, deployment)
}

func (d *deploymentResourceReader) ReadStatusForObject(ctx context.Context, deployment *unstructured.Unstructured) *event.ResourceStatus {
	identifier := toIdentifier(deployment)

	replicaSetStatuses, err := d.StatusForGeneratedResources(ctx, d.RsStatusReader, deployment,
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
	res, err := d.computeStatusFunc(deployment)
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

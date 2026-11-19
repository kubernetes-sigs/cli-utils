// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func NewDeploymentResourceReader(mapper meta.RESTMapper, rsStatusReader resourceTypeStatusReader) engine.StatusReader {
	return &baseStatusReader{
		mapper: mapper,
		resourceStatusReader: &deploymentResourceReader{
			mapper:         mapper,
			rsStatusReader: rsStatusReader,
		},
	}
}

// deploymentResourceReader is a resourceTypeStatusReader that can fetch Deployment
// resources from the cluster, knows how to find any ReplicaSets belonging to the
// Deployment, and compute status for the deployment.
type deploymentResourceReader struct {
	mapper meta.RESTMapper

	// rsStatusReader is the implementation of the resourceTypeStatusReader
	// the knows how to compute the status for ReplicaSets.
	rsStatusReader resourceTypeStatusReader
}

var _ resourceTypeStatusReader = &deploymentResourceReader{}

func (d *deploymentResourceReader) Supports(gk schema.GroupKind) bool {
	return gk == appsv1.SchemeGroupVersion.WithKind("Deployment").GroupKind()
}

func (d *deploymentResourceReader) ReadStatusForObject(ctx context.Context, reader engine.ClusterReader,
	deployment *unstructured.Unstructured) (*event.ResourceStatus, error) {
	identifier := object.UnstructuredToObjMetadata(deployment)

	replicaSetStatuses, err := statusForGeneratedResources(ctx, d.mapper, reader, d.rsStatusReader, deployment,
		appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind(), "spec", "selector")
	if err != nil {
		return errResourceToResourceStatus(err, deployment)
	}

	// Currently this engine just uses the status library for computing
	// status for the deployment. But we do have the status and state for all
	// ReplicaSets and Pods in the ObservedReplicaSets data structure, so the
	// rules can be improved to take advantage of this information.
	res, err := status.Compute(deployment)
	if err != nil {
		return errResourceToResourceStatus(err, deployment, replicaSetStatuses...)
	}

	return &event.ResourceStatus{
		Identifier:         identifier,
		Status:             res.Status,
		Resource:           deployment,
		Message:            explainPendingTermination(res, deployment, replicaSetStatuses),
		GeneratedResources: replicaSetStatuses,
	}, nil
}

const deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"

// explainPendingTermination adds the status of the new pods to the "Pending termination"
// message. Deployment status only counts non-terminating pods, so this message also shows
// up when old pods are kept running while a new pod is not yet available.
func explainPendingTermination(res *status.Result, deployment *unstructured.Unstructured,
	replicaSetStatuses []*event.ResourceStatus) string {
	revision := deployment.GetAnnotations()[deploymentRevisionAnnotation]
	if res.Status != status.InProgressStatus || !hasExtraPodsCondition(res) || revision == "" {
		return res.Message
	}
	for _, rs := range replicaSetStatuses {
		if rs == nil || rs.Resource == nil || (rs.Status != status.InProgressStatus && rs.Status != status.FailedStatus) ||
			rs.Resource.GetAnnotations()[deploymentRevisionAnnotation] != revision ||
			!isOwnedBy(rs.Resource, deployment.GetUID()) {
			continue
		}
		return fmt.Sprintf("%s. New pods %s-*: %s", res.Message, rs.Identifier.Name, rs.Message)
	}
	return res.Message
}

func hasExtraPodsCondition(res *status.Result) bool {
	for _, c := range res.Conditions {
		if c.Type == status.ConditionReconciling && c.Reason == "ExtraPods" {
			return true
		}
	}
	return false
}

func isOwnedBy(obj *unstructured.Unstructured, uid types.UID) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

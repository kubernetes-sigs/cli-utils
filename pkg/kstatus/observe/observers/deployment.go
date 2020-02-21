package observers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/observer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

func NewDeploymentObserver(reader observer.ClusterReader, mapper meta.RESTMapper, rsObserver observer.ResourceObserver) observer.ResourceObserver {
	return &DeploymentObserver{
		BaseObserver: BaseObserver{
			Reader:            reader,
			Mapper:            mapper,
			computeStatusFunc: status.Compute,
		},
		RsObserver: rsObserver,
	}
}

// DeploymentObserver is an observer that can fetch Deployment resources
// from the cluster, knows how to find any ReplicaSets belonging to the
// Deployment, and compute status for the deployment.
type DeploymentObserver struct {
	BaseObserver

	RsObserver observer.ResourceObserver
}

func (d *DeploymentObserver) Observe(ctx context.Context, identifier wait.ResourceIdentifier) *event.ObservedResource {
	deployment, err := d.LookupResource(ctx, identifier)
	if err != nil {
		return d.handleObservedResourceError(identifier, err)
	}
	return d.ObserveObject(ctx, deployment)
}

func (d *DeploymentObserver) ObserveObject(ctx context.Context, deployment *unstructured.Unstructured) *event.ObservedResource {
	identifier := toIdentifier(deployment)

	observedReplicaSets, err := d.ObserveGeneratedResources(ctx, d.RsObserver, deployment,
		appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind(), "spec", "selector")
	if err != nil {
		return &event.ObservedResource{
			Identifier: identifier,
			Status:     status.UnknownStatus,
			Resource:   deployment,
			Error:      err,
		}
	}

	// Currently this observer just uses the status library for computing
	// status for the deployment. But we do have the status and state for all
	// ReplicaSets and Pods in the ObservedReplicaSets data structure, so the
	// rules can be improved to take advantage of this information.
	res, err := d.computeStatusFunc(deployment)
	if err != nil {
		return &event.ObservedResource{
			Identifier:         identifier,
			Status:             status.UnknownStatus,
			Error:              err,
			GeneratedResources: observedReplicaSets,
		}
	}

	return &event.ObservedResource{
		Identifier:         identifier,
		Status:             res.Status,
		Resource:           deployment,
		Message:            res.Message,
		GeneratedResources: observedReplicaSets,
	}
}

package observers

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/observer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

func NewStatefulSetObserver(reader observer.ClusterReader, mapper meta.RESTMapper, podObserver observer.ResourceObserver) observer.ResourceObserver {
	return &StatefulSetObserver{
		BaseObserver: BaseObserver{
			Reader:            reader,
			Mapper:            mapper,
			computeStatusFunc: status.Compute,
		},
		PodObserver: podObserver,
	}
}

// StatefulObserver is an observer that can fetch StatefulSet resources
// from the cluster, knows how to find any Pods belonging to the
// StatefulSet, and compute status for the StatefulSet.
type StatefulSetObserver struct {
	BaseObserver

	PodObserver observer.ResourceObserver
}

func (s *StatefulSetObserver) Observe(ctx context.Context, identifier wait.ResourceIdentifier) *event.ObservedResource {
	statefulSet, err := s.LookupResource(ctx, identifier)
	if err != nil {
		return s.handleObservedResourceError(identifier, err)
	}
	return s.ObserveObject(ctx, statefulSet)
}

func (s *StatefulSetObserver) ObserveObject(ctx context.Context, statefulSet *unstructured.Unstructured) *event.ObservedResource {
	identifier := toIdentifier(statefulSet)

	observedPods, err := s.ObserveGeneratedResources(ctx, s.PodObserver, statefulSet,
		v1.SchemeGroupVersion.WithKind("Pod").GroupKind(), "spec", "selector")
	if err != nil {
		return &event.ObservedResource{
			Identifier: identifier,
			Status:     status.UnknownStatus,
			Resource:   statefulSet,
			Error:      err,
		}
	}

	res, err := s.computeStatusFunc(statefulSet)
	if err != nil {
		return &event.ObservedResource{
			Identifier:         identifier,
			Status:             status.UnknownStatus,
			Error:              err,
			GeneratedResources: observedPods,
		}
	}

	return &event.ObservedResource{
		Identifier:         identifier,
		Status:             res.Status,
		Resource:           statefulSet,
		Message:            res.Message,
		GeneratedResources: observedPods,
	}
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package observe

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/aggregator"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/observer"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/observers"
	"sigs.k8s.io/cli-utils/pkg/kstatus/observe/reader"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewStatusObserver creates a new StatusObserver using the given reader and mapper. The StatusObserver
// will use the client for all calls to the cluster.
func NewStatusObserver(reader client.Reader, mapper meta.RESTMapper, useCache bool) *StatusObserver {
	return &StatusObserver{
		observer: &observer.Observer{
			Reader: reader,
			Mapper: mapper,

			ObserversFactoryFunc:  createObservers,
			ReaderFactoryFunc:     readerFactoryFunc(useCache),
			AggregatorFactoryFunc: aggregatorFactoryFunc(),
		},
	}
}

// StatusObserver provides functionality for polling a cluster for status for a set of resources.
type StatusObserver struct {
	observer *observer.Observer
}

// Observe will create a new statusObserverRunner that will poll all the resources provided and report their status
// back on the event channel returned. The statusObserverRunner can be cancelled at any time by cancelling the
// context passed in.
// If stopOnCompleted is set to true, then the runner will stop observing the resources when the StatusAggregator
// determines that all resources has been fully reconciled. If this is set to false, the observer will keep running
// until cancelled.
// If useCache is set to true, the observer will fetch all resources needed using LIST calls before each polling
// cycle. The observers responsible for computing status will rely on the cached data. This can dramatically reduce
// the number of calls against the API server.
func (s *StatusObserver) Observe(ctx context.Context, identifiers []wait.ResourceIdentifier, options Options) <-chan event.Event {
	return s.observer.Observe(ctx, identifiers, options.PollInterval, options.StopOnCompleted)
}

// Options contains the different parameters that can be used to adjust the
// behavior of the StatusObserver.
type Options struct {
	// StopOnCompleted defines whether the observer should stop polling and close the
	// event channel when the Aggregator implementation considers all resources to have reached
	// the desired status.
	StopOnCompleted bool

	// PollInterval defines how often the Observer should poll the cluster for the latest
	// state of the resources.
	PollInterval time.Duration
}

// createObservers creates an instance of all the observers. This includes a set of observers for
// a particular GroupKind, and a default observer used for all resource types that does not have
// a specific observers.
// TODO: We should consider making the registration more automatic instead of having to create each of them
// here. Also, it might be worth creating them on demand.
func createObservers(reader observer.ClusterReader, mapper meta.RESTMapper) (map[schema.GroupKind]observer.ResourceObserver, observer.ResourceObserver) {
	defaultObserver := observers.NewGenericObserver(reader, mapper)

	replicaSetObserver := observers.NewReplicaSetObserver(reader, mapper, defaultObserver)
	deploymentObserver := observers.NewDeploymentObserver(reader, mapper, replicaSetObserver)
	statefulSetObserver := observers.NewStatefulSetObserver(reader, mapper, defaultObserver)

	resourceObservers := map[schema.GroupKind]observer.ResourceObserver{
		appsv1.SchemeGroupVersion.WithKind("Deployment").GroupKind():  deploymentObserver,
		appsv1.SchemeGroupVersion.WithKind("StatefulSet").GroupKind(): statefulSetObserver,
		appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind():  replicaSetObserver,
	}

	return resourceObservers, defaultObserver
}

// readerFactoryFunc returns a factory function for creating an instance of a ClusterReader.
// This function is used by the StatusObserver to create a ClusterReader for each StatusObserverRunner.
// The decision for which implementation of the ClusterReader interface that should be used are
// decided here rather than based on information passed in to the factory function. Thus, the decision
// for which implementation is decided when the StatusObserver is created.
func readerFactoryFunc(useCache bool) observer.ReaderFactoryFunc {
	return func(r client.Reader, mapper meta.RESTMapper, identifiers []wait.ResourceIdentifier) (observer.ClusterReader, error) {
		if useCache {
			return reader.NewCachingClusterReader(r, mapper, identifiers)
		}
		return &reader.DirectClusterReader{Reader: r}, nil
	}
}

// aggregatorFactoryFunc returns a factory function for creating an instance of the
// StatusAggregator interface. Currently there is only one implementation.
func aggregatorFactoryFunc() observer.AggregatorFactoryFunc {
	return func(identifiers []wait.ResourceIdentifier) observer.StatusAggregator {
		return aggregator.NewAllCurrentOrNotFoundStatusAggregator(identifiers)
	}
}

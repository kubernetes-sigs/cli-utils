// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package polling

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/aggregator"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/clusterreader"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/statusreaders"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewStatusPoller creates a new StatusPoller using the given clusterreader and mapper. The StatusPoller
// will use the client for all calls to the cluster.
func NewStatusPoller(reader client.Reader, mapper meta.RESTMapper, useCache bool) *StatusPoller {
	return &StatusPoller{
		engine: &engine.PollerEngine{
			Reader: reader,
			Mapper: mapper,

			StatusReadersFactoryFunc: createStatusReaders,
			ClusterReaderFactoryFunc: clusterReaderFactoryFunc(useCache),
			AggregatorFactoryFunc:    aggregatorFactoryFunc(),
		},
	}
}

// StatusPoller provides functionality for polling a cluster for status for a set of resources.
type StatusPoller struct {
	engine *engine.PollerEngine
}

// Poll will create a new statusPollerRunner that will poll all the resources provided and report their status
// back on the event channel returned. The statusPollerRunner can be cancelled at any time by cancelling the
// context passed in.
// If stopOnCompleted is set to true, then the runner will stop polling the resources when the StatusAggregator
// determines that all resources has been fully reconciled. If this is set to false, the engine will keep running
// until cancelled.
// If useCache is set to true, the engine will fetch all resources needed using LIST calls before each polling
// cycle. The statusreaders responsible for computing status will rely on the cached data. This can dramatically reduce
// the number of calls against the API server.
func (s *StatusPoller) Poll(ctx context.Context, identifiers []wait.ResourceIdentifier, options Options) <-chan event.Event {
	return s.engine.Poll(ctx, identifiers, options.PollInterval, options.StopOnCompleted)
}

// Options contains the different parameters that can be used to adjust the
// behavior of the StatusPoller.
type Options struct {
	// StopOnCompleted defines whether the engine should stop polling and close the
	// event channel when the Aggregator implementation considers all resources to have reached
	// the desired status.
	StopOnCompleted bool

	// PollInterval defines how often the PollerEngine should poll the cluster for the latest
	// state of the resources.
	PollInterval time.Duration
}

// createStatusReaders creates an instance of all the statusreaders. This includes a set of statusreaders for
// a particular GroupKind, and a default engine used for all resource types that does not have
// a specific statusreaders.
// TODO: We should consider making the registration more automatic instead of having to create each of them
// here. Also, it might be worth creating them on demand.
func createStatusReaders(reader engine.ClusterReader, mapper meta.RESTMapper) (map[schema.GroupKind]engine.StatusReader, engine.StatusReader) {
	defaultStatusReader := statusreaders.NewGenericStatusReader(reader, mapper)

	replicaSetStatusReader := statusreaders.NewReplicaSetStatusReader(reader, mapper, defaultStatusReader)
	deploymentStatusReader := statusreaders.NewDeploymentResourceReader(reader, mapper, replicaSetStatusReader)
	statefulSetStatusReader := statusreaders.NewStatefulSetResourceReader(reader, mapper, defaultStatusReader)

	statusReaders := map[schema.GroupKind]engine.StatusReader{
		appsv1.SchemeGroupVersion.WithKind("Deployment").GroupKind():  deploymentStatusReader,
		appsv1.SchemeGroupVersion.WithKind("StatefulSet").GroupKind(): statefulSetStatusReader,
		appsv1.SchemeGroupVersion.WithKind("ReplicaSet").GroupKind():  replicaSetStatusReader,
	}

	return statusReaders, defaultStatusReader
}

// clusterReaderFactoryFunc returns a factory function for creating an instance of a ClusterReader.
// This function is used by the StatusPoller to create a ClusterReader for each StatusPollerRunner.
// The decision for which implementation of the ClusterReader interface that should be used are
// decided here rather than based on information passed in to the factory function. Thus, the decision
// for which implementation is decided when the StatusPoller is created.
func clusterReaderFactoryFunc(useCache bool) engine.ClusterReaderFactoryFunc {
	return func(r client.Reader, mapper meta.RESTMapper, identifiers []wait.ResourceIdentifier) (engine.ClusterReader, error) {
		if useCache {
			return clusterreader.NewCachingClusterReader(r, mapper, identifiers)
		}
		return &clusterreader.DirectClusterReader{Reader: r}, nil
	}
}

// aggregatorFactoryFunc returns a factory function for creating an instance of the
// StatusAggregator interface. Currently there is only one implementation.
func aggregatorFactoryFunc() engine.AggregatorFactoryFunc {
	return func(identifiers []wait.ResourceIdentifier) engine.StatusAggregator {
		return aggregator.NewAllCurrentOrNotFoundStatusAggregator(identifiers)
	}
}

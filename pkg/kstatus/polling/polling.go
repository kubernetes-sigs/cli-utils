// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package polling

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/clusterreader"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/statusreaders"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewStatusPoller creates a new StatusPoller using the given clusterreader and mapper. The StatusPoller
// will use the client for all calls to the cluster.
func NewStatusPoller(reader client.Reader, mapper meta.RESTMapper, customStatusReaders []engine.StatusReader) *StatusPoller {
	var statusReaders []engine.StatusReader

	statusReaders = append(statusReaders, customStatusReaders...)

	srs, defaultStatusReader := createStatusReaders(mapper)
	statusReaders = append(statusReaders, srs...)

	return &StatusPoller{
		engine: &engine.PollerEngine{
			Reader:              reader,
			Mapper:              mapper,
			DefaultStatusReader: defaultStatusReader,
			StatusReaders:       statusReaders,
		},
	}
}

// NewStatusPollerFromFactory creates a new StatusPoller instance from the
// passed in factory.
func NewStatusPollerFromFactory(f cmdutil.Factory, statusReaders []engine.StatusReader) (*StatusPoller, error) {
	config, err := f.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("error getting RESTConfig: %w", err)
	}

	mapper, err := f.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("error getting RESTMapper: %w", err)
	}

	c, err := client.New(config, client.Options{Scheme: scheme.Scheme, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("error creating client: %w", err)
	}

	return NewStatusPoller(c, mapper, statusReaders), nil
}

// StatusPoller provides functionality for polling a cluster for status for a set of resources.
type StatusPoller struct {
	engine *engine.PollerEngine
}

// Poll will create a new statusPollerRunner that will poll all the resources provided and report their status
// back on the event channel returned. The statusPollerRunner can be cancelled at any time by cancelling the
// context passed in.
func (s *StatusPoller) Poll(ctx context.Context, identifiers object.ObjMetadataSet, options Options) <-chan event.Event {
	return s.engine.Poll(ctx, identifiers, engine.Options{
		PollInterval:             options.PollInterval,
		ClusterReaderFactoryFunc: clusterReaderFactoryFunc(options.UseCache),
	})
}

// Options defines the levers available for tuning the behavior of the
// StatusPoller.
type Options struct {
	// PollInterval defines how often the PollerEngine should poll the cluster for the latest
	// state of the resources.
	PollInterval time.Duration

	// UseCache defines whether the ClusterReader should use LIST calls to fetch
	// all needed resources before each polling cycle. If this is set to false,
	// then each resource will be fetched when needed with GET calls.
	UseCache bool
}

// createStatusReaders creates an instance of all the statusreaders. This includes a set of statusreaders for
// a particular GroupKind, and a default engine used for all resource types that does not have
// a specific statusreaders.
// TODO: We should consider making the registration more automatic instead of having to create each of them
// here. Also, it might be worth creating them on demand.
func createStatusReaders(mapper meta.RESTMapper) ([]engine.StatusReader, engine.StatusReader) {
	defaultStatusReader := statusreaders.NewGenericStatusReader(mapper, status.Compute)

	replicaSetStatusReader := statusreaders.NewReplicaSetStatusReader(mapper, defaultStatusReader)
	deploymentStatusReader := statusreaders.NewDeploymentResourceReader(mapper, replicaSetStatusReader)
	statefulSetStatusReader := statusreaders.NewStatefulSetResourceReader(mapper, defaultStatusReader)

	statusReaders := []engine.StatusReader{
		deploymentStatusReader,
		statefulSetStatusReader,
		replicaSetStatusReader,
	}

	return statusReaders, defaultStatusReader
}

// clusterReaderFactoryFunc returns a factory function for creating an instance of a ClusterReader.
// This function is used by the StatusPoller to create a ClusterReader for each StatusPollerRunner.
// The decision for which implementation of the ClusterReader interface that should be used are
// decided here rather than based on information passed in to the factory function. Thus, the decision
// for which implementation is decided when the StatusPoller is created.
func clusterReaderFactoryFunc(useCache bool) engine.ClusterReaderFactoryFunc {
	return func(r client.Reader, mapper meta.RESTMapper, identifiers object.ObjMetadataSet) (engine.ClusterReader, error) {
		if useCache {
			return clusterreader.NewCachingClusterReader(r, mapper, identifiers)
		}
		return &clusterreader.DirectClusterReader{Reader: r}, nil
	}
}

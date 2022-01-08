// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// NewDestroyer returns a new destroyer. It will set up the ApplyOptions and
// PruneOptions which are responsible for capturing any command line flags.
// It currently requires IOStreams, but this is a legacy from when
// the ApplyOptions were responsible for printing progress. This is now
// handled by a separate printer with the KubectlPrinterAdapter bridging
// between the two.
func NewDestroyer(factory cmdutil.Factory, invClient inventory.InventoryClient) (*Destroyer, error) {
	pruner, err := prune.NewPruner(factory, invClient)
	if err != nil {
		return nil, fmt.Errorf("error setting up PruneOptions: %w", err)
	}
	statusPoller, err := polling.NewStatusPollerFromFactory(factory, []engine.StatusReader{})
	if err != nil {
		return nil, err
	}
	return &Destroyer{
		pruner:       pruner,
		StatusPoller: statusPoller,
		factory:      factory,
		invClient:    invClient,
	}, nil
}

// Destroyer performs the step of grabbing all the previous inventory objects and
// prune them. This also deletes all the previous inventory objects
type Destroyer struct {
	pruner       *prune.Pruner
	StatusPoller poller.Poller
	factory      cmdutil.Factory
	invClient    inventory.InventoryClient
}

type DestroyerOptions struct {
	// InventoryPolicy defines the inventory policy of apply.
	InventoryPolicy inventory.InventoryPolicy

	// DryRunStrategy defines whether changes should actually be performed,
	// or if it is just talk and no action.
	DryRunStrategy common.DryRunStrategy

	// DeleteTimeout defines how long we should wait for resources
	// to be fully deleted.
	DeleteTimeout time.Duration

	// DeletePropagationPolicy defines the deletion propagation policy
	// that should be used. If this is not provided, the default is to
	// use the Background policy.
	DeletePropagationPolicy metav1.DeletionPropagation

	// EmitStatusEvents defines whether status events should be
	// emitted on the eventChannel to the caller.
	EmitStatusEvents bool

	// PollInterval defines how often we should poll for the status
	// of resources.
	PollInterval time.Duration
}

func setDestroyerDefaults(o *DestroyerOptions) {
	if o.PollInterval == time.Duration(0) {
		o.PollInterval = poller.DefaultPollInterval
	}
	if o.DeletePropagationPolicy == "" {
		o.DeletePropagationPolicy = metav1.DeletePropagationBackground
	}
}

// Run performs the destroy step. Passes the inventory object. This
// happens asynchronously on progress and any errors are reported
// back on the event channel.
func (d *Destroyer) Run(ctx context.Context, inv inventory.InventoryInfo, options DestroyerOptions) <-chan event.Event {
	eventChannel := make(chan event.Event)
	setDestroyerDefaults(&options)
	go func() {
		defer close(eventChannel)
		// Retrieve the objects to be deleted from the cluster. Second parameter is empty
		// because no local objects returns all inventory objects for deletion.
		emptyLocalObjs := object.UnstructuredSet{}
		deleteObjs, err := d.pruner.GetPruneObjs(inv, emptyLocalObjs, prune.Options{
			DryRunStrategy: options.DryRunStrategy,
		})
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		mapper, err := d.factory.ToRESTMapper()
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		klog.V(4).Infoln("destroyer building task queue...")
		taskBuilder := &solver.TaskQueueBuilder{
			Pruner:    d.pruner,
			Factory:   d.factory,
			Mapper:    mapper,
			InvClient: d.invClient,
			Destroy:   true,
		}
		opts := solver.Options{
			Prune:                  true,
			PruneTimeout:           options.DeleteTimeout,
			DryRunStrategy:         options.DryRunStrategy,
			PrunePropagationPolicy: options.DeletePropagationPolicy,
		}
		deleteFilters := []filter.ValidationFilter{
			filter.PreventRemoveFilter{},
			filter.InventoryPolicyFilter{
				Inv:       inv,
				InvPolicy: options.InventoryPolicy,
			},
		}
		// Build the ordered set of tasks to execute.
		taskQueue, err := taskBuilder.
			AppendPruneWaitTasks(deleteObjs, deleteFilters, opts).
			AppendDeleteInvTask(inv, options.DryRunStrategy).
			Build()
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		// Send event to inform the caller about the resources that
		// will be pruned.
		eventChannel <- event.Event{
			Type: event.InitType,
			InitEvent: event.InitEvent{
				ActionGroups: taskQueue.ToActionGroups(),
			},
		}
		// Create a new TaskStatusRunner to execute the taskQueue.
		klog.V(4).Infoln("destroyer building TaskStatusRunner...")
		deleteIds := object.UnstructuredSetToObjMetadataSet(deleteObjs)
		resourceCache := cache.NewResourceCacheMap()
		runner := taskrunner.NewTaskStatusRunner(deleteIds, d.StatusPoller, resourceCache)
		klog.V(4).Infoln("destroyer running TaskStatusRunner...")
		// TODO(seans): Make the poll interval configurable like the applier.
		err = runner.Run(ctx, taskQueue.ToChannel(), eventChannel, taskrunner.Options{
			UseCache:         true,
			PollInterval:     options.PollInterval,
			EmitStatusEvents: options.EmitStatusEvents,
		})
		if err != nil {
			handleError(eventChannel, err)
			return
		}
	}()
	return eventChannel
}

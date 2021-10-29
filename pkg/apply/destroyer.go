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
	applyerror "sigs.k8s.io/cli-utils/pkg/apply/error"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// NewDestroyer returns a new destroyer. It will set up the ApplyOptions and
// PruneOptions which are responsible for capturing any command line flags.
// It currently requires IOStreams, but this is a legacy from when
// the ApplyOptions were responsible for printing progress. This is now
// handled by a separate printer with the KubectlPrinterAdapter bridging
// between the two.
func NewDestroyer(factory cmdutil.Factory, invClient inventory.InventoryClient, statusPoller poller.Poller) (*Destroyer, error) {
	pruneOpts, err := prune.NewPruneOptions(factory, invClient)
	if err != nil {
		return nil, fmt.Errorf("error setting up PruneOptions: %w", err)
	}
	return &Destroyer{
		pruneOptions: pruneOpts,
		statusPoller: statusPoller,
		factory:      factory,
		invClient:    invClient,
	}, nil
}

// Destroyer performs the step of grabbing all the previous inventory objects and
// prune them. This also deletes all the previous inventory objects
type Destroyer struct {
	pruneOptions *prune.PruneOptions
	statusPoller poller.Poller
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

	// ContinueOnError defines whether to continue on error.
	ContinueOnError bool
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
func (d *Destroyer) Run(inv inventory.InventoryInfo, options DestroyerOptions) <-chan event.Event {
	eventChannel := make(chan event.Event)
	setDestroyerDefaults(&options)
	go func() {
		defer close(eventChannel)
		// Retrieve the objects to be deleted from the cluster. Second parameter is empty
		// because no local objects returns all inventory objects for deletion.
		emptyLocalObjs := object.UnstructuredSet{}
		deleteObjs, err := d.pruneOptions.GetPruneObjs(inv, emptyLocalObjs, prune.Options{
			DryRunStrategy: options.DryRunStrategy,
		})
		if err != nil {
			applyerror.HandleError(eventChannel, err)
			return
		}
		mapper, err := d.factory.ToRESTMapper()
		if err != nil {
			applyerror.HandleError(eventChannel, err)
			return
		}
		klog.V(4).Infoln("destroyer building task queue...")
		taskBuilder := &solver.TaskQueueBuilder{
			PruneOptions: d.pruneOptions,
			Factory:      d.factory,
			Mapper:       mapper,
			InvClient:    d.invClient,
			Destroy:      true,
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
			applyerror.HandleError(eventChannel, err)
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
		deleteIds := object.UnstructuredsToObjMetasOrDie(deleteObjs)
		resourceCache := cache.NewResourceCacheMap()
		runner := taskrunner.NewTaskStatusRunner(deleteIds, d.statusPoller, resourceCache)
		klog.V(4).Infoln("destroyer running TaskStatusRunner...")
		// TODO(seans): Make the poll interval configurable like the applier.
		err = runner.Run(context.Background(), taskQueue.ToChannel(), eventChannel, taskrunner.Options{
			UseCache:         true,
			PollInterval:     options.PollInterval,
			EmitStatusEvents: options.EmitStatusEvents,
			ContinueOnError:  options.ContinueOnError,
		})
		if err != nil {
			applyerror.HandleError(eventChannel, err)
		}
	}()
	return eventChannel
}

// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"time"

	"github.com/go-errors/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/provider"
	"sigs.k8s.io/cli-utils/pkg/util/factory"
)

// NewDestroyer returns a new destroyer. It will set up the ApplyOptions and
// PruneOptions which are responsible for capturing any command line flags.
// It currently requires IOStreams, but this is a legacy from when
// the ApplyOptions were responsible for printing progress. This is now
// handled by a separate printer with the KubectlPrinterAdapter bridging
// between the two.
func NewDestroyer(provider provider.Provider) *Destroyer {
	return &Destroyer{
		PruneOptions: prune.NewPruneOptions(),
		provider:     provider,
	}
}

// Destroyer performs the step of grabbing all the previous inventory objects and
// prune them. This also deletes all the previous inventory objects
type Destroyer struct {
	provider     provider.Provider
	StatusPoller poller.Poller
	PruneOptions *prune.PruneOptions
	invClient    inventory.InventoryClient
}

type DestroyerOption struct {
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
}

// Initialize sets up the Destroyer for actually doing an destroy against
// a cluster. This involves validating command line inputs and configuring
// clients for communicating with the cluster.
func (d *Destroyer) Initialize() error {
	statusPoller, err := factory.NewStatusPoller(d.provider.Factory())
	if err != nil {
		return errors.WrapPrefix(err, "error creating status poller", 1)
	}
	d.StatusPoller = statusPoller
	invClient, err := d.provider.InventoryClient()
	if err != nil {
		return errors.WrapPrefix(err, "error creating inventory client", 1)
	}
	d.invClient = invClient
	err = d.PruneOptions.Initialize(d.provider.Factory(), invClient)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up PruneOptions", 1)
	}
	d.PruneOptions.Destroy = true
	return nil
}

// Run performs the destroy step. Passes the inventory object. This
// happens asynchronously on progress and any errors are reported
// back on the event channel.
func (d *Destroyer) Run(inv inventory.InventoryInfo, option *DestroyerOption) <-chan event.Event {
	eventChannel := make(chan event.Event)
	d.invClient.SetDryRunStrategy(option.DryRunStrategy)
	go func() {
		defer close(eventChannel)
		// Retrieve the objects to be deleted from the cluster. Second parameter is empty
		// because no local objects returns all inventory objects for deletion.
		emptyLocalObjs := []*unstructured.Unstructured{}
		deleteObjs, err := d.PruneOptions.GetPruneObjs(inv, emptyLocalObjs)
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		mapper, err := d.provider.Factory().ToRESTMapper()
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		klog.V(4).Infoln("destroyer building task queue...")
		taskBuilder := &solver.TaskQueueBuilder{
			PruneOptions: d.PruneOptions,
			Factory:      d.provider.Factory(),
			Mapper:       mapper,
			InvClient:    d.invClient,
		}
		opts := solver.Options{
			Prune:                  true,
			PruneTimeout:           option.DeleteTimeout,
			DryRunStrategy:         option.DryRunStrategy,
			PrunePropagationPolicy: option.DeletePropagationPolicy,
		}
		deleteFilters := []filter.ValidationFilter{
			filter.PreventRemoveFilter{},
			filter.InventoryPolicyFilter{
				Inv:       inv,
				InvPolicy: option.InventoryPolicy,
			},
		}
		// Build the ordered set of tasks to execute.
		taskQueue := taskBuilder.
			AppendPruneWaitTasks(deleteObjs, deleteFilters, opts).
			AppendDeleteInvTask(inv).
			Build()
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
		deleteIds := object.UnstructuredsToObjMetas(deleteObjs)
		runner := taskrunner.NewTaskStatusRunner(deleteIds, d.StatusPoller)
		klog.V(4).Infoln("destroyer running TaskStatusRunner...")
		// TODO(seans): Make the poll interval configurable like the applier.
		err = runner.Run(context.Background(), taskQueue.ToChannel(), eventChannel, taskrunner.Options{
			UseCache:         true,
			PollInterval:     poller.DefaultPollInterval,
			EmitStatusEvents: true,
		})
		if err != nil {
			handleError(eventChannel, err)
		}
	}()
	return eventChannel
}

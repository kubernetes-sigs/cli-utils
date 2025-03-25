// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/watcher"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
)

// Destroyer performs the step of grabbing all the previous inventory objects and
// prune them. This also deletes all the previous inventory objects
type Destroyer struct {
	pruner        *prune.Pruner
	statusWatcher watcher.StatusWatcher
	invClient     inventory.Client
	mapper        meta.RESTMapper
	client        dynamic.Interface
	openAPIGetter discovery.OpenAPISchemaInterface
	infoHelper    info.Helper
}

type DestroyerOptions struct {
	// InventoryPolicy defines the inventory policy of apply.
	InventoryPolicy inventory.Policy

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

	// ValidationPolicy defines how to handle invalid objects.
	ValidationPolicy validation.Policy
}

func setDestroyerDefaults(o *DestroyerOptions) {
	if o.DeletePropagationPolicy == "" {
		o.DeletePropagationPolicy = metav1.DeletePropagationBackground
	}
}

// Run performs the destroy step. Passes the inventory object. This
// happens asynchronously on progress and any errors are reported
// back on the event channel.
func (d *Destroyer) Run(ctx context.Context, invInfo inventory.Info, options DestroyerOptions) <-chan event.Event {
	eventChannel := make(chan event.Event)
	setDestroyerDefaults(&options)
	go func() {
		defer close(eventChannel)
		inv, err := d.invClient.Get(ctx, invInfo, inventory.GetOptions{})
		if apierrors.IsNotFound(err) {
			inv, err = d.invClient.NewInventory(invInfo)
		}
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		if inv.Info().GetID() != invInfo.GetID() {
			handleError(eventChannel, fmt.Errorf("expected inventory object to have inventory-id %q but got %q",
				invInfo.GetID(), inv.Info().GetID()))
			return
		}

		// Retrieve the objects to be deleted from the cluster. Second parameter is empty
		// because no local objects returns all inventory objects for deletion.
		emptyLocalObjs := object.UnstructuredSet{}
		deleteObjs, err := d.pruner.GetPruneObjs(ctx, inv, emptyLocalObjs, prune.Options{
			DryRunStrategy: options.DryRunStrategy,
		})
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		// Validate the resources to make sure we catch those problems early
		// before anything has been updated in the cluster.
		vCollector := &validation.Collector{}
		validator := &validation.Validator{
			Collector: vCollector,
			Mapper:    d.mapper,
		}
		validator.Validate(deleteObjs)

		// Build a TaskContext for passing info between tasks
		resourceCache := cache.NewResourceCacheMap()
		taskContext := taskrunner.NewTaskContext(ctx, eventChannel, resourceCache)

		klog.V(4).Infoln("destroyer building task queue...")
		deleteFilters := []filter.ValidationFilter{
			filter.PreventRemoveFilter{},
			filter.InventoryPolicyPruneFilter{
				Inv:       invInfo,
				InvPolicy: options.InventoryPolicy,
			},
			filter.DependencyFilter{
				TaskContext:       taskContext,
				ActuationStrategy: actuation.ActuationStrategyDelete,
				DryRunStrategy:    options.DryRunStrategy,
			},
		}
		taskBuilder := &solver.TaskQueueBuilder{
			Pruner:        d.pruner,
			DynamicClient: d.client,
			OpenAPIGetter: d.openAPIGetter,
			InfoHelper:    d.infoHelper,
			Mapper:        d.mapper,
			InvClient:     d.invClient,
			Inventory:     inv,
			Collector:     vCollector,
			PruneFilters:  deleteFilters,
		}
		opts := solver.Options{
			Destroy:                true,
			Prune:                  true,
			DryRunStrategy:         options.DryRunStrategy,
			PrunePropagationPolicy: options.DeletePropagationPolicy,
			PruneTimeout:           options.DeleteTimeout,
			InventoryPolicy:        options.InventoryPolicy,
		}

		// Build the ordered set of tasks to execute.
		taskQueue := taskBuilder.
			WithPruneObjects(deleteObjs).
			Build(taskContext, opts)

		klog.V(4).Infof("validation errors: %d", len(vCollector.Errors))
		klog.V(4).Infof("invalid objects: %d", len(vCollector.InvalidIDs))

		// Handle validation errors
		switch options.ValidationPolicy {
		case validation.ExitEarly:
			err = vCollector.ToError()
			if err != nil {
				handleError(eventChannel, err)
				return
			}
		case validation.SkipInvalid:
			for _, err := range vCollector.Errors {
				handleValidationError(eventChannel, err)
			}
		default:
			handleError(eventChannel, fmt.Errorf("invalid ValidationPolicy: %q", options.ValidationPolicy))
			return
		}

		// Register invalid objects to be retained in the inventory, if present.
		for _, id := range vCollector.InvalidIDs {
			taskContext.AddInvalidObject(id)
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
		deleteIDs := object.UnstructuredSetToObjMetadataSet(deleteObjs)
		statusWatcher := d.statusWatcher
		// Disable watcher for dry runs
		if opts.DryRunStrategy.ClientOrServerDryRun() {
			statusWatcher = watcher.BlindStatusWatcher{}
		}
		runner := taskrunner.NewTaskStatusRunner(deleteIDs, statusWatcher)
		klog.V(4).Infoln("destroyer running TaskStatusRunner...")
		err = runner.Run(ctx, taskContext, taskQueue.ToChannel(), taskrunner.Options{
			EmitStatusEvents: options.EmitStatusEvents,
		})
		if err != nil {
			handleError(eventChannel, err)
			return
		}
	}()
	return eventChannel
}

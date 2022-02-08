// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/cache"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
)

// NewDestroyer returns a new destroyer. It will set up the ApplyOptions and
// PruneOptions which are responsible for capturing any command line flags.
// It currently requires IOStreams, but this is a legacy from when
// the ApplyOptions were responsible for printing progress. This is now
// handled by a separate printer with the KubectlPrinterAdapter bridging
// between the two.
func NewDestroyer(factory cmdutil.Factory, invClient inventory.Client) (*Destroyer, error) {
	pruner, err := prune.NewPruner(factory, invClient)
	if err != nil {
		return nil, fmt.Errorf("error setting up PruneOptions: %w", err)
	}
	statusPoller, err := polling.NewStatusPollerFromFactory(factory, polling.Options{})
	if err != nil {
		return nil, err
	}
	client, err := factory.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("error getting dynamic client: %v", err)
	}
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("error getting rest mapper: %v", err)
	}
	invObjManager := &inventory.ObjectManager{
		Mapper:        mapper,
		DynamicClient: client,
	}
	return &Destroyer{
		pruner:        pruner,
		StatusPoller:  statusPoller,
		factory:       factory,
		invClient:     invClient,
		client:        client,
		mapper:        mapper,
		invObjManager: invObjManager,
	}, nil
}

// Destroyer performs the step of grabbing all the previous inventory objects and
// prune them. This also deletes all the previous inventory objects
type Destroyer struct {
	pruner        *prune.Pruner
	StatusPoller  poller.Poller
	factory       cmdutil.Factory
	invClient     inventory.Client
	client        dynamic.Interface
	mapper        meta.RESTMapper
	invObjManager *inventory.ObjectManager
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

	// ValidationPolicy defines how to handle invalid objects.
	ValidationPolicy validation.Policy
}

func setDestroyerDefaults(o *DestroyerOptions) {
	if o.PollInterval == time.Duration(0) {
		o.PollInterval = defaultPollInterval
	}
	if o.DeletePropagationPolicy == "" {
		o.DeletePropagationPolicy = metav1.DeletePropagationBackground
	}
}

// prepareObjects returns the set of objects to prune or an error if one occurred.
func (d *Destroyer) prepareObjects(
	ctx context.Context,
	invInfo inventory.InventoryInfo,
) (pruneObjs, invObjs object.UnstructuredSet, err error) {
	// Load the inventory from storage
	inv, err := d.invClient.Load(invInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load inventory: %w", err)
	}
	if inv == nil {
		return nil, nil, fmt.Errorf("inventory not found: %v", invInfo)
	}

	if d.invObjManager == nil {
		return nil, nil, fmt.Errorf("missing inventory client")
	}

	// Get the inventory objects from the cluster (exclude NotFound).
	invObjs, err = d.invObjManager.GetSpecObjects(ctx, inv)
	if err != nil {
		return nil, nil, err
	}

	if d.invObjManager == nil {
		return nil, nil, fmt.Errorf("missing inventory object manager")
	}

	// delete all inventory objects (deep copy)
	pruneObjs = make(object.UnstructuredSet, len(invObjs))
	for i, obj := range invObjs {
		pruneObjs[i] = obj.DeepCopy()
	}

	return pruneObjs, invObjs, nil
}

// Run performs the destroy step. Passes the inventory object. This
// happens asynchronously on progress and any errors are reported
// back on the event channel.
func (d *Destroyer) Run(ctx context.Context, invInfo inventory.InventoryInfo, options DestroyerOptions) <-chan event.Event {
	eventChannel := make(chan event.Event)
	setDestroyerDefaults(&options)
	go func() {
		defer close(eventChannel)

		// Decide which objects to prune
		deleteObjs, _, err := d.prepareObjects(ctx, invInfo)
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		klog.V(4).Infof("calculated %d delete objs", len(deleteObjs))

		// Validate the resources to make sure we catch those problems early
		// before anything has been updated in the cluster.
		vCollector := &validation.Collector{}
		validator := &validation.Validator{
			Collector: vCollector,
			Mapper:    d.mapper,
		}
		validator.Validate(deleteObjs)

		klog.V(4).Infoln("destroyer building task queue...")
		dynamicClient, err := d.factory.DynamicClient()
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		taskBuilder := &solver.TaskQueueBuilder{
			Pruner:        d.pruner,
			DynamicClient: dynamicClient,
			OpenAPIGetter: d.factory.OpenAPIGetter(),
			InfoHelper:    info.NewInfoHelper(d.mapper, d.factory.UnstructuredClientForMapping),
			Mapper:        d.mapper,
			InvClient:     d.invClient,
			Destroy:       true,
			Collector:     vCollector,
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
				InvInfo:   invInfo,
				InvPolicy: options.InventoryPolicy,
			},
		}

		// Build the ordered set of tasks to execute.
		taskQueue := taskBuilder.
			AppendPruneWaitTasks(deleteObjs, deleteFilters, opts).
			AppendDeleteInvTask(options.DryRunStrategy).
			Build()

		klog.V(4).Infof("validation errors: %d", len(vCollector.Errors))
		klog.V(4).Infof("invalid objects: %d", len(vCollector.InvalidIds))

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

		// Build a TaskContext for passing info between tasks
		resourceCache := cache.NewResourceCacheMap()
		taskContext := taskrunner.NewTaskContext(eventChannel, resourceCache)

		// initialize inventory in the task context
		inv := taskContext.InventoryManager().Inventory()
		inv.SetGroupVersionKind(d.invClient.GroupVersionKind())
		inv.SetName(invInfo.Name)
		inv.SetNamespace(invInfo.Namespace)
		inventory.SetInventoryLabel(inv, invInfo.ID)

		// Register invalid objects to be retained in the inventory, if present.
		for _, id := range vCollector.InvalidIds {
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
		deleteIds := object.UnstructuredSetToObjMetadataSet(deleteObjs)
		runner := taskrunner.NewTaskStatusRunner(deleteIds, d.StatusPoller)
		klog.V(4).Infoln("destroyer running TaskStatusRunner...")
		err = runner.Run(ctx, taskContext, taskQueue.ToChannel(), taskrunner.Options{
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

// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/ordering"
)

// NewApplier returns a new Applier.
func NewApplier(factory cmdutil.Factory, invClient inventory.InventoryClient, statusPoller poller.Poller) (*Applier, error) {
	pruneOpts, err := prune.NewPruner(factory, invClient)
	if err != nil {
		return nil, err
	}
	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}
	return &Applier{
		pruner:       pruneOpts,
		statusPoller: statusPoller,
		factory:      factory,
		invClient:    invClient,
		infoHelper:   info.NewInfoHelper(mapper, factory),
	}, nil
}

// Applier performs the step of applying a set of resources into a cluster,
// conditionally waits for all of them to be fully reconciled and finally
// performs prune to clean up any resources that has been deleted.
// The applier performs its function by executing a list queue of tasks,
// each of which is one of the steps in the process of applying a set
// of resources to the cluster. The actual execution of these tasks are
// handled by a StatusRunner. So the taskqueue is effectively a
// specification that is executed by the StatusRunner. Based on input
// parameters and/or the set of resources that needs to be applied to the
// cluster, different sets of tasks might be needed.
type Applier struct {
	pruner       *prune.Pruner
	statusPoller poller.Poller
	factory      cmdutil.Factory
	invClient    inventory.InventoryClient
	infoHelper   info.InfoHelper
}

// prepareObjects returns the set of objects to apply and to prune or
// an error if one occurred.
func (a *Applier) prepareObjects(
	ctx context.Context,
	localInv inventory.InventoryInfo,
	localObjs []*unstructured.Unstructured,
	opts Options,
) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	if localInv == nil {
		return nil, nil, fmt.Errorf("the local inventory can't be nil")
	}
	if err := inventory.ValidateNoInventory(localObjs); err != nil {
		return nil, nil, err
	}
	// Add the inventory annotation to the resources being applied.
	for _, localObj := range localObjs {
		inventory.AddInventoryIDAnnotation(localObj, localInv)
	}
	// If the inventory uses the Name strategy and an inventory ID is provided,
	// verify that the existing inventory object (if there is one) has an ID
	// label that matches.
	// TODO(seans): This inventory id validation should happen in destroy and status.
	if localInv.Strategy() == inventory.NameStrategy && localInv.ID() != "" {
		prevInvObjs, err := a.invClient.GetClusterInventoryObjs(localInv)
		if err != nil {
			return nil, nil, err
		}
		if len(prevInvObjs) > 1 {
			panic(fmt.Errorf("found %d inv objects with Name strategy", len(prevInvObjs)))
		}
		if len(prevInvObjs) == 1 {
			invObj := prevInvObjs[0]
			val := invObj.GetLabels()[common.InventoryLabel]
			if val != localInv.ID() {
				return nil, nil, fmt.Errorf("inventory-id of inventory object in cluster doesn't match provided id %q", localInv.ID())
			}
		}
	}
	pruneObjs, err := a.pruner.GetPruneObjs(
		ctx,
		localInv,
		localObjs,
		opts.PruneOptions(),
	)
	if err != nil {
		return nil, nil, err
	}
	sort.Sort(ordering.SortableUnstructureds(localObjs))
	return localObjs, pruneObjs, nil
}

// Run performs the Apply step. This happens asynchronously with updates
// on progress and any errors are reported back on the event channel.
// Cancelling the operation or setting timeout on how long to Wait
// for it complete can be done with the passed in context.
// Note: There isn't currently any way to interrupt the operation
// before all the given resources have been applied to the cluster. Any
// cancellation or timeout will only affect how long we Wait for the
// resources to become current.
func (a *Applier) Run(ctx context.Context, invInfo inventory.InventoryInfo, objects []*unstructured.Unstructured, opts Options) <-chan event.Event {
	klog.V(4).Infof("apply run for %d objects", len(objects))
	eventChannel := make(chan event.Event)
	setDefaults(&opts)
	go func() {
		defer close(eventChannel)

		client, err := a.factory.DynamicClient()
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		mapper, err := a.factory.ToRESTMapper()
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		// Validate the resources to make sure we catch those problems early
		// before anything has been updated in the cluster.
		if err := (&object.Validator{
			Mapper: mapper,
		}).Validate(objects); err != nil {
			handleError(eventChannel, err)
			return
		}

		applyObjs, pruneObjs, err := a.prepareObjects(ctx, invInfo, objects, opts)
		if err != nil {
			handleError(eventChannel, err)
			return
		}
		klog.V(4).Infof("calculated %d apply objs; %d prune objs", len(applyObjs), len(pruneObjs))

		// Fetch the queue (channel) of tasks that should be executed.
		klog.V(4).Infoln("applier building task queue...")
		taskBuilder := &solver.TaskQueueBuilder{
			Pruner:     a.pruner,
			Factory:    a.factory,
			InfoHelper: a.infoHelper,
			Mapper:     mapper,
			InvClient:  a.invClient,
			Destroy:    false, // DO NOT remove pruned resources from inventory
		}
		solverOpts := opts.SolverOptions()
		// Build list of apply validation filters.
		applyFilters := []filter.ValidationFilter{
			filter.InventoryPolicyApplyFilter{
				Client:    client,
				Mapper:    mapper,
				Inv:       invInfo,
				InvPolicy: opts.InventoryPolicy,
			},
		}
		// Build list of prune validation filters.
		pruneFilters := []filter.ValidationFilter{
			filter.PreventRemoveFilter{},
			filter.InventoryPolicyFilter{
				Inv:       invInfo,
				InvPolicy: opts.InventoryPolicy,
			},
			filter.LocalNamespacesFilter{
				LocalNamespaces: localNamespaces(invInfo, object.UnstructuredsToObjMetasOrDie(objects)),
			},
		}

		// Build the ordered set of tasks to execute.
		taskBuilder.AppendInvAddTask(invInfo, applyObjs, opts.DryRunStrategy)
		taskBuilder.AppendApplyWaitTasks(applyObjs, applyFilters, solverOpts)
		if !opts.NoPrune {
			taskBuilder.AppendPruneWaitTasks(pruneObjs, pruneFilters, solverOpts)
		}
		taskBuilder.AppendInvSetTask(invInfo, opts.DryRunStrategy)
		taskQueue, err := taskBuilder.Build()
		if err != nil {
			handleError(eventChannel, err)
		}
		// Send event to inform the caller about the resources that
		// will be applied/pruned.
		eventChannel <- event.Event{
			Type: event.InitType,
			InitEvent: event.InitEvent{
				ActionGroups: taskQueue.ToActionGroups(),
			},
		}
		// Create a new TaskStatusRunner to execute the taskQueue.
		klog.V(4).Infoln("applier building TaskStatusRunner...")
		allIds := object.UnstructuredsToObjMetasOrDie(append(applyObjs, pruneObjs...))
		runner := taskrunner.NewTaskStatusRunner(allIds, a.statusPoller)
		klog.V(4).Infoln("applier running TaskStatusRunner...")
		err = runner.Run(ctx, taskQueue.ToChannel(), eventChannel, opts.TaskRunnerOptions())
		if err != nil {
			handleError(eventChannel, err)
		}
	}()
	return eventChannel
}

type Options struct {
	// Encapsulates the fields for server-side apply.
	ServerSideOptions common.ServerSideOptions

	// ReconcileTimeout defines whether the applier should wait
	// until all applied resources have been reconciled, and if so,
	// how long to wait.
	ReconcileTimeout time.Duration

	// PollInterval defines how often we should poll for the status
	// of resources.
	PollInterval time.Duration

	// EmitStatusEvents defines whether status events should be
	// emitted on the eventChannel to the caller.
	EmitStatusEvents bool

	// NoPrune defines whether pruning of previously applied
	// objects should happen after apply.
	NoPrune bool

	// DryRunStrategy defines whether changes should actually be performed,
	// or if it is just talk and no action.
	DryRunStrategy common.DryRunStrategy

	// DeletionPropagationPolicy defines the deletion propagation policy
	// that should be used for pruning. If this is not provided, the
	// default is to use the Background policy.
	DeletionPropagationPolicy metav1.DeletionPropagation

	// DeleteTimeout defines whether we should wait for all resources
	// to be fully deleted after pruning, and if so, how long we should
	// wait.
	DeleteTimeout time.Duration

	// InventoryPolicy defines the inventory policy of apply.
	InventoryPolicy inventory.InventoryPolicy
}

func (o Options) PruneOptions() prune.Options {
	return prune.Options{
		DryRunStrategy: o.DryRunStrategy,
		DeleteTimeout:  o.DeleteTimeout,
	}
}

func (o Options) SolverOptions() solver.Options {
	return solver.Options{
		ServerSideOptions:       o.ServerSideOptions,
		ReconcileTimeout:        o.ReconcileTimeout,
		DryRunStrategy:          o.DryRunStrategy,
		DeletePropagationPolicy: o.DeletionPropagationPolicy,
		DeleteTimeout:           o.DeleteTimeout,
		InventoryPolicy:         o.InventoryPolicy,
	}
}

func (o Options) TaskRunnerOptions() taskrunner.Options {
	return taskrunner.Options{
		UseCache:         true,
		PollInterval:     o.PollInterval,
		EmitStatusEvents: o.EmitStatusEvents,
	}
}

// setDefaults set the options to the default values if they
// have not been provided.
func setDefaults(o *Options) {
	if o.PollInterval == time.Duration(0) {
		o.PollInterval = poller.DefaultPollInterval
	}
	if o.DeletionPropagationPolicy == "" {
		o.DeletionPropagationPolicy = metav1.DeletePropagationBackground
	}
}

func handleError(eventChannel chan event.Event, err error) {
	eventChannel <- event.Event{
		Type: event.ErrorType,
		ErrorEvent: event.ErrorEvent{
			Err: err,
		},
	}
}

// localNamespaces stores a set of strings of all the namespaces
// for the passed non cluster-scoped localObjs, plus the namespace
// of the passed inventory object. This is used to skip deleting
// namespaces which have currently applied objects in them.
func localNamespaces(localInv inventory.InventoryInfo, localObjs []object.ObjMetadata) sets.String {
	namespaces := sets.NewString()
	for _, obj := range localObjs {
		namespace := strings.ToLower(obj.Namespace)
		if namespace != "" {
			namespaces.Insert(namespace)
		}
	}
	invNamespace := strings.ToLower(localInv.Namespace())
	if invNamespace != "" {
		namespaces.Insert(invNamespace)
	}
	return namespaces
}

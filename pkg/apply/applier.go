// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/go-errors/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/ordering"
	"sigs.k8s.io/cli-utils/pkg/provider"
	"sigs.k8s.io/cli-utils/pkg/util/factory"
)

// newApplier returns a new Applier. It will set up the ApplyOptions and
// StatusOptions which are responsible for capturing any command line flags.
// It currently requires IOStreams, but this is a legacy from when
// the ApplyOptions were responsible for printing progress. This is now
// handled by a separate printer with the KubectlPrinterAdapter bridging
// between the two.
func NewApplier(provider provider.Provider) *Applier {
	a := &Applier{
		PruneOptions: prune.NewPruneOptions(),
		provider:     provider,
		infoHelper:   info.NewInfoHelper(provider.Factory()),
	}
	return a
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
	provider provider.Provider

	PruneOptions *prune.PruneOptions
	StatusPoller poller.Poller
	invClient    inventory.InventoryClient
	infoHelper   info.InfoHelper
}

// Initialize sets up the Applier for actually doing an apply against
// a cluster. This involves validating command line inputs and configuring
// clients for communicating with the cluster.
func (a *Applier) Initialize() error {
	var err error
	a.invClient, err = a.provider.InventoryClient()
	if err != nil {
		return err
	}
	err = a.PruneOptions.Initialize(a.provider.Factory(), a.invClient)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up PruneOptions", 1)
	}

	statusPoller, err := factory.NewStatusPoller(a.provider.Factory())
	if err != nil {
		return errors.WrapPrefix(err, "error creating resolver", 1)
	}
	a.StatusPoller = statusPoller
	return nil
}

// prepareObjects returns ResourceObjects or an error if one occurred.
func (a *Applier) prepareObjects(localInv inventory.InventoryInfo, localObjs []*unstructured.Unstructured) (*ResourceObjects, error) {
	klog.V(4).Infof("applier preparing %d objects", len(localObjs))
	if localInv == nil {
		return nil, fmt.Errorf("the local inventory can't be nil")
	}
	if err := inventory.ValidateNoInventory(localObjs); err != nil {
		return nil, err
	}

	// If the inventory uses the Name strategy and an inventory ID is provided,
	// verify that the existing inventory object (if there is one) has an ID
	// label that matches.
	if localInv.Strategy() == inventory.NameStrategy && localInv.ID() != "" {
		invObjs, err := a.invClient.GetClusterInventoryObjs(localInv)
		if err != nil {
			return nil, err
		}

		if len(invObjs) > 1 {
			panic(fmt.Errorf("found %d inv objects with Name strategy", len(invObjs)))
		}

		if len(invObjs) == 1 {
			invObj := invObjs[0]
			val := invObj.GetLabels()[common.InventoryLabel]
			if val != localInv.ID() {
				return nil, fmt.Errorf("inventory-id of inventory object in cluster doesn't match provided id %q", localInv.ID())
			}
		}
	}

	// Retrieve previous inventory objects to calculate prune ids.
	prevInv, err := a.invClient.GetClusterObjs(localInv)
	if err != nil {
		return nil, err
	}
	locals := object.UnstructuredsToObjMetas(localObjs)
	pruneIds := object.SetDiff(prevInv, locals)
	klog.V(4).Infof("applier calculated %d prune objects", len(pruneIds))

	// Sort order for applied resources.
	sort.Sort(ordering.SortableUnstructureds(localObjs))

	return &ResourceObjects{
		LocalInv:  localInv,
		Resources: localObjs,
		PruneIds:  pruneIds,
		PrevInv:   prevInv,
	}, nil
}

// ResourceObjects contains information about the resources that
// will be applied and the existing inventories used to determine
// resources that should be pruned.
type ResourceObjects struct {
	LocalInv  inventory.InventoryInfo
	Resources []*unstructured.Unstructured
	PruneIds  []object.ObjMetadata
	PrevInv   []object.ObjMetadata
}

// ObjsForApply returns the unstructured representation for all the resources
// that should be applied, not including the inventory object. The
// resources will be in sorted order.
func (r *ResourceObjects) ObjsForApply() []*unstructured.Unstructured {
	return r.Resources
}

// Inventory returns the unstructured representation of the inventory object.
func (r *ResourceObjects) Inventory() inventory.InventoryInfo {
	return r.LocalInv
}

// IdsForApply returns the Ids for all resources that should be applied,
// including the inventory object.
func (r *ResourceObjects) IdsForApply() []object.ObjMetadata {
	var ids []object.ObjMetadata
	for _, obj := range r.ObjsForApply() {
		ids = append(ids, object.UnstructuredToObjMeta(obj))
	}
	return ids
}

// IdsForPrune returns the Ids for all resources that should
// be pruned.
func (r *ResourceObjects) IdsForPrune() []object.ObjMetadata {
	return r.PruneIds
}

// IdsForPrevInv returns the Ids for the previous inventory. These
// Ids reference the objects managed by the inventory object which
// are already in the cluster.
func (r *ResourceObjects) IdsForPrevInv() []object.ObjMetadata {
	return r.PrevInv
}

// AllIds returns the Ids for all resources that are relevant. This
// includes resources that will be applied or pruned.
func (r *ResourceObjects) AllIds() []object.ObjMetadata {
	return append(r.IdsForApply(), r.IdsForPrune()...)
}

// Run performs the Apply step. This happens asynchronously with updates
// on progress and any errors are reported back on the event channel.
// Cancelling the operation or setting timeout on how long to Wait
// for it complete can be done with the passed in context.
// Note: There sn't currently any way to interrupt the operation
// before all the given resources have been applied to the cluster. Any
// cancellation or timeout will only affect how long we Wait for the
// resources to become current.
func (a *Applier) Run(ctx context.Context, invInfo inventory.InventoryInfo, objects []*unstructured.Unstructured, options Options) <-chan event.Event {
	klog.V(4).Infof("apply run for %d objects", len(objects))
	eventChannel := make(chan event.Event)
	setDefaults(&options)
	a.invClient.SetDryRunStrategy(options.DryRunStrategy) // client shared with prune, so sets dry-run for prune too.
	go func() {
		defer close(eventChannel)

		// This provides us with a slice of all the objects that will be
		// applied to the cluster. This takes care of ordering resources
		// and handling the inventory object.
		resourceObjects, err := a.prepareObjects(invInfo, objects)
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		mapper, err := a.provider.Factory().ToRESTMapper()
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		// Fetch the queue (channel) of tasks that should be executed.
		klog.V(4).Infoln("applier building task queue...")
		taskQueue := (&solver.TaskQueueSolver{
			PruneOptions: a.PruneOptions,
			Factory:      a.provider.Factory(),
			InfoHelper:   a.infoHelper,
			Mapper:       mapper,
			InvClient:    a.invClient,
		}).BuildTaskQueue(resourceObjects, solver.Options{
			ServerSideOptions:      options.ServerSideOptions,
			ReconcileTimeout:       options.ReconcileTimeout,
			Prune:                  !options.NoPrune,
			DryRunStrategy:         options.DryRunStrategy,
			PrunePropagationPolicy: options.PrunePropagationPolicy,
			PruneTimeout:           options.PruneTimeout,
			InventoryPolicy:        options.InventoryPolicy,
		})

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
		runner := taskrunner.NewTaskStatusRunner(resourceObjects.AllIds(), a.StatusPoller)
		klog.V(4).Infoln("applier running TaskStatusRunner...")
		err = runner.Run(ctx, taskQueue.ToChannel(), eventChannel, taskrunner.Options{
			PollInterval:     options.PollInterval,
			UseCache:         true,
			EmitStatusEvents: options.EmitStatusEvents,
		})
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

	// PrunePropagationPolicy defines the deletion propagation policy
	// that should be used for pruning. If this is not provided, the
	// default is to use the Background policy.
	PrunePropagationPolicy metav1.DeletionPropagation

	// PruneTimeout defines whether we should wait for all resources
	// to be fully deleted after pruning, and if so, how long we should
	// wait.
	PruneTimeout time.Duration

	// InventoryPolicy defines the inventory policy of apply.
	InventoryPolicy inventory.InventoryPolicy
}

// setDefaults set the options to the default values if they
// have not been provided.
func setDefaults(o *Options) {
	if o.PollInterval == time.Duration(0) {
		o.PollInterval = 2 * time.Second
	}
	if o.PrunePropagationPolicy == metav1.DeletionPropagation("") {
		o.PrunePropagationPolicy = metav1.DeletePropagationBackground
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

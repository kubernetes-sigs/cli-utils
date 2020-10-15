// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"sort"
	"time"

	"github.com/go-errors/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
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
func NewApplier(provider provider.Provider, ioStreams genericclioptions.IOStreams) *Applier {
	a := &Applier{
		PruneOptions: prune.NewPruneOptions(),
		provider:     provider,
		ioStreams:    ioStreams,
	}
	a.infoHelperFactoryFunc = a.infoHelperFactory
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
	provider  provider.Provider
	ioStreams genericclioptions.IOStreams

	PruneOptions *prune.PruneOptions
	StatusPoller poller.Poller
	invClient    inventory.InventoryClient

	// infoHelperFactoryFunc is used to create a new instance of the
	// InfoHelper. It is defined here so we can override it in unit tests.
	infoHelperFactoryFunc func() info.InfoHelper
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

// infoHelperFactory returns a new instance of the InfoHelper.
func (a *Applier) infoHelperFactory() info.InfoHelper {
	return info.NewInfoHelper(a.provider.Factory())
}

// prepareObjects merges the currently applied objects into the
// set of stored objects in the cluster inventory. In the process, it
// calculates the set of objects to be pruned (pruneIds), and orders the
// resources for the subsequent apply. Returns the sorted resources to
// apply as well as the objects for the prune, or an error if one occurred.
func (a *Applier) prepareObjects(infos []*resource.Info) (*ResourceObjects, error) {
	localInv, localInfos, err := inventory.SplitInfos(infos)
	if err != nil {
		return nil, err
	}

	// Ensures the namespace exists before applying the inventory object into it.
	if invNamespace := inventoryNamespaceInSet(localInv, localInfos); invNamespace != nil {
		if err = a.invClient.ApplyInventoryNamespace(invNamespace); err != nil {
			return nil, err
		}
	}

	currentObjs, err := object.InfosToObjMetas(localInfos)
	if err != nil {
		return nil, err
	}
	// returns the objects (pruneIds) to prune after apply. The prune
	// algorithm requires stopping if the merge is not successful. Otherwise,
	// the stored objects in inventory could become inconsistent.
	pruneIds, err := a.invClient.Merge(localInv, currentObjs)
	if err != nil {
		return nil, err
	}
	// Sort order for applied resources.
	sort.Sort(ordering.SortableInfos(localInfos))

	return &ResourceObjects{
		LocalInv:  localInv,
		Resources: localInfos,
		PruneIds:  pruneIds,
	}, nil
}

// ResourceObjects contains information about the resources that
// will be applied and the existing inventories used to determine
// resources that should be pruned.
type ResourceObjects struct {
	LocalInv  *resource.Info
	Resources []*resource.Info
	PruneIds  []object.ObjMetadata
}

// InfosForApply returns the infos representation for all the resources
// that should be applied, including the inventory object. The
// resources will be in sorted order.
func (r *ResourceObjects) InfosForApply() []*resource.Info {
	return r.Resources
}

func (r *ResourceObjects) InfosForPrune() []*resource.Info {
	return append([]*resource.Info{r.LocalInv}, r.Resources...)
}

// IdsForApply returns the Ids for all resources that should be applied,
// including the inventory object.
func (r *ResourceObjects) IdsForApply() []object.ObjMetadata {
	var ids []object.ObjMetadata
	for _, info := range r.InfosForApply() {
		id, err := object.InfoToObjMeta(info)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// IdsForPrune returns the Ids for all resources that should
// be pruned.
func (r *ResourceObjects) IdsForPrune() []object.ObjMetadata {
	return r.PruneIds
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
func (a *Applier) Run(ctx context.Context, objects []*resource.Info, options Options) <-chan event.Event {
	eventChannel := make(chan event.Event)
	setDefaults(&options)
	a.invClient.SetDryRunStrategy(options.DryRunStrategy) // client shared with prune, so sets dry-run for prune too.
	go func() {
		defer close(eventChannel)

		// This provides us with a slice of all the objects that will be
		// applied to the cluster. This takes care of ordering resources
		// and handling the inventory object.
		resourceObjects, err := a.prepareObjects(objects)
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		mapper, err := a.provider.ToRESTMapper()
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		// Fetch the queue (channel) of tasks that should be executed.
		taskQueue := (&solver.TaskQueueSolver{
			PruneOptions: a.PruneOptions,
			Factory:      a.provider.Factory(),
			InfoHelper:   a.infoHelperFactoryFunc(),
			Mapper:       mapper,
		}).BuildTaskQueue(resourceObjects, solver.Options{
			ReconcileTimeout:       options.ReconcileTimeout,
			Prune:                  !options.NoPrune,
			DryRunStrategy:         options.DryRunStrategy,
			PrunePropagationPolicy: options.PrunePropagationPolicy,
			PruneTimeout:           options.PruneTimeout,
		})

		// Send event to inform the caller about the resources that
		// will be applied/pruned.
		eventChannel <- event.Event{
			Type: event.InitType,
			InitEvent: event.InitEvent{
				ResourceGroups: []event.ResourceGroup{
					{
						Action:      event.ApplyAction,
						Identifiers: resourceObjects.IdsForApply(),
					},
					{
						Action:      event.PruneAction,
						Identifiers: resourceObjects.IdsForPrune(),
					},
				},
			},
		}

		// Create a new TaskStatusRunner to execute the taskQueue.
		runner := taskrunner.NewTaskStatusRunner(resourceObjects.AllIds(), a.StatusPoller)
		err = runner.Run(ctx, taskQueue, eventChannel, taskrunner.Options{
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

// inventoryNamespaceInSet returns the the namespace the passed inventory
// object will be applied to, or nil if this namespace object does not exist
// in the passed slice "infos" or the inventory object is cluster-scoped.
func inventoryNamespaceInSet(inv *resource.Info, infos []*resource.Info) *resource.Info {
	if inv == nil || inv.Object == nil {
		return nil
	}
	invAcc, _ := meta.Accessor(inv.Object)
	invNamespace := invAcc.GetNamespace()

	for _, info := range infos {
		acc, _ := meta.Accessor(info.Object)
		gvk := info.Object.GetObjectKind().GroupVersionKind()
		if gvk == object.CoreV1Namespace && acc.GetName() == invNamespace {
			return info
		}
	}
	return nil
}

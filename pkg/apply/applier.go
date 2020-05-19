// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/poller"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/solver"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newApplier returns a new Applier. It will set up the ApplyOptions and
// StatusOptions which are responsible for capturing any command line flags.
// It currently requires IOStreams, but this is a legacy from when
// the ApplyOptions were responsible for printing progress. This is now
// handled by a separate printer with the KubectlPrinterAdapter bridging
// between the two.
func NewApplier(factory util.Factory, ioStreams genericclioptions.IOStreams) *Applier {
	applyOptions := apply.NewApplyOptions(ioStreams)
	a := &Applier{
		ApplyOptions: applyOptions,
		// VisitedUids keeps track of the unique identifiers for all
		// currently applied objects. Used to calculate prune set.
		PruneOptions: prune.NewPruneOptions(applyOptions.VisitedUids),
		factory:      factory,
		ioStreams:    ioStreams,
	}
	a.previousInventoriesFunc = a.previousInventories
	a.infoHelperFactoryFunc = a.infoHelperFactory
	a.manifestReaderFunc = a.readObjects
	a.InventoryFactoryFunc = prune.WrapInventoryObj

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
	factory   util.Factory
	ioStreams genericclioptions.IOStreams

	ApplyOptions *apply.ApplyOptions
	PruneOptions *prune.PruneOptions
	StatusPoller poller.Poller

	//TODO(mortent): See if we can come up with a better structure here
	// so we don't need these functions to facilitate testing.
	//
	// previousInventoriesFunc is used to fetch the existing inventories
	// from the apiserver. It is defined here so we can override it in
	// unit tests.
	previousInventoriesFunc func(label, namespace string) ([]*resource.Info, error)
	// infoHelperFactoryFunc is used to create a new instance of the
	// InfoHelper. It is defined here so we can override it in unit tests.
	infoHelperFactoryFunc func() info.InfoHelper
	// manifestReaderFunc is used to read manifests from files or from
	// stdin. It is defined here so we can override it in unit tests.
	manifestReaderFunc func() ([]*resource.Info, error)
	// InventoryFactoryFunc wraps and returns an interface for the
	// object which will load and store the inventory.
	InventoryFactoryFunc func(*resource.Info) prune.Inventory
}

// Initialize sets up the Applier for actually doing an apply against
// a cluster. This involves validating command line inputs and configuring
// clients for communicating with the cluster.
func (a *Applier) Initialize(cmd *cobra.Command, paths []string) error {
	fileNameFlags, err := common.DemandOneDirectory(paths)
	if err != nil {
		return err
	}
	a.ApplyOptions.DeleteFlags.FileNameFlags = &fileNameFlags
	err = a.ApplyOptions.Complete(a.factory, cmd)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up ApplyOptions", 1)
	}
	a.ApplyOptions.PostProcessorFn = nil // Turn off the default kubectl pruning
	err = a.PruneOptions.Initialize(a.factory)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up PruneOptions", 1)
	}

	statusPoller, err := a.newStatusPoller()
	if err != nil {
		return errors.WrapPrefix(err, "error creating resolver", 1)
	}
	a.StatusPoller = statusPoller
	return nil
}

// SetFlags configures the command line flags needed for apply and
// status. This is a temporary solution as we should separate the configuration
// of cobra flags from the Applier.
func (a *Applier) SetFlags(cmd *cobra.Command) error {
	a.ApplyOptions.DeleteFlags.AddFlags(cmd)
	for _, flag := range []string{"kustomize", "filename", "recursive"} {
		err := cmd.Flags().MarkHidden(flag)
		if err != nil {
			return err
		}
	}
	a.ApplyOptions.RecordFlags.AddFlags(cmd)
	_ = cmd.Flags().MarkHidden("record")
	_ = cmd.Flags().MarkHidden("cascade")
	_ = cmd.Flags().MarkHidden("force")
	_ = cmd.Flags().MarkHidden("grace-period")
	_ = cmd.Flags().MarkHidden("timeout")
	_ = cmd.Flags().MarkHidden("wait")
	a.ApplyOptions.Overwrite = true
	return nil
}

// newStatusPoller sets up a new StatusPoller for computing status. The configuration
// needed for the poller is taken from the Factory.
func (a *Applier) newStatusPoller() (poller.Poller, error) {
	config, err := a.factory.ToRESTConfig()
	if err != nil {
		return nil, errors.WrapPrefix(err, "error getting RESTConfig", 1)
	}

	mapper, err := a.factory.ToRESTMapper()
	if err != nil {
		return nil, errors.WrapPrefix(err, "error getting RESTMapper", 1)
	}

	c, err := client.New(config, client.Options{Scheme: scheme.Scheme, Mapper: mapper})
	if err != nil {
		return nil, errors.WrapPrefix(err, "error creating client", 1)
	}

	return polling.NewStatusPoller(c, mapper), nil
}

// previousInventories returns the infos for all the inventory objects
// in the cluster.
func (a *Applier) previousInventories(label, namespace string) ([]*resource.Info, error) {
	return a.PruneOptions.RetrievePreviousInventoryObjects(label, namespace)
}

// infoHelperFactory returns a new instance of the InfoHelper.
func (a *Applier) infoHelperFactory() info.InfoHelper {
	return info.NewInfoHelper(a.factory, a.ApplyOptions.Namespace)
}

// readObjects reads the manifests from disk with the local flag set on
// the Builder. This means that the resulting Info resources does not have
// the mapping and client properties set. We are doing it this way because
// it allows us to read in both CRDs and CRs in the same set without
// errors.
func (a *Applier) readObjects() ([]*resource.Info, error) {
	r := a.factory.NewBuilder().
		Local().
		Unstructured().
		Schema(a.ApplyOptions.Validator).
		ContinueOnError().
		NamespaceParam(a.ApplyOptions.Namespace).DefaultNamespace().
		FilenameParam(a.ApplyOptions.EnforceNamespace, &a.ApplyOptions.DeleteOptions.FilenameOptions).
		LabelSelectorParam(a.ApplyOptions.Selector).
		Flatten().
		Do()
	if err := r.Err(); err != nil {
		return nil, err
	}
	return r.Infos()
}

// setNamespaces verifies that every namespaced resource has the namespace
// set, and if one does not, it will set the namespace to the provided
// defaultNamespace.
// This implementation will check each resource (that doesn't already have
// the namespace set) on whether it is namespace or cluster scoped. It does
// this by first checking the RESTMapper, and it there is not match there,
// it will look for CRDs in the provided Infos.
func (a *Applier) setNamespaces(infos []*resource.Info, defaultNamespace string,
	enforceNamespace bool) error {
	var crdInfos []*resource.Info

	// find any crds in the set of resources.
	for _, inf := range infos {
		if solver.IsCRD(inf) {
			crdInfos = append(crdInfos, inf)
		}
	}

	mapper, err := a.factory.ToRESTMapper()
	if err != nil {
		return err
	}
	for _, inf := range infos {
		accessor, _ := meta.Accessor(inf.Object)
		// if the resource already has the namespace set, we don't
		// need to do anything
		if ns := accessor.GetNamespace(); ns != "" {
			if enforceNamespace && ns != defaultNamespace {
				return fmt.Errorf("the namespace from the provided object %q "+
					"does not match the namespace %q. You must pass '--namespace=%s' to perform this operation",
					ns, defaultNamespace, ns)
			}
			continue
		}

		gk := inf.Object.GetObjectKind().GroupVersionKind().GroupKind()
		mapping, err := mapper.RESTMapping(gk)

		if err != nil && !meta.IsNoMatchError(err) {
			return err
		}

		if err == nil {
			// If we find a mapping for the resource type in the RESTMapper,
			// we just use it.
			if mapping.Scope == meta.RESTScopeNamespace {
				// This means the resource does not have the namespace set,
				// but it is a namespaced resource. So we set the namespace
				// to the provided default value.
				inf.Namespace = defaultNamespace
				accessor.SetNamespace(defaultNamespace)
			}
			continue
		}

		// If we get here, it means the resource does not have the namespace
		// set and we didn't find the resource type in the RESTMapper. As
		// a last try, we look at all the CRDS that are part of the set and
		// see if we get a match on the resource type. If so, we can determine
		// from the CRD whether the resource type is cluster-scoped or
		// namespace-scoped. If it is the latter, we set the namespace
		// to the provided default.
		var scope string
		for _, crdInf := range crdInfos {
			u, _ := crdInf.Object.(*unstructured.Unstructured)
			group, _, _ := unstructured.NestedString(u.Object, "spec", "group")
			kind, _, _ := unstructured.NestedString(u.Object, "spec", "names", "kind")
			if gk.Kind == kind && gk.Group == group {
				scope, _, _ = unstructured.NestedString(u.Object, "spec", "scope")
			}
		}

		switch scope {
		case "":
			return fmt.Errorf("can't find scope for resource %s %s", gk.String(), accessor.GetName())
		case "Cluster":
			continue
		case "Namespaced":
			inf.Namespace = defaultNamespace
			accessor.SetNamespace(defaultNamespace)
		}
	}

	return nil
}

// readAndPrepareObjects reads the resources that should be applied,
// handles ordering of resources and sets up the inventory object
// based on the provided inventory object template.
func (a *Applier) readAndPrepareObjects() (*ResourceObjects, error) {
	infos, err := a.manifestReaderFunc()
	if err != nil {
		return nil, err
	}

	err = a.setNamespaces(infos, a.ApplyOptions.Namespace, a.ApplyOptions.EnforceNamespace)
	if err != nil {
		return nil, err
	}

	resources, invs := splitInfos(infos)

	if len(invs) == 0 {
		return nil, prune.NoInventoryObjError{}
	}
	if len(invs) > 1 {
		return nil, prune.MultipleInventoryObjError{
			InventoryObjectTemplates: invs,
		}
	}

	inv := a.InventoryFactoryFunc(invs[0])
	inventoryObject, err := prune.CreateInventoryObj(inv, resources)
	if err != nil {
		return nil, err
	}

	// Fetch the inventory label used for the current set of resources.
	inventoryLabel, err := prune.RetrieveInventoryLabel(inventoryObject.Object)
	if err != nil {
		return nil, err
	}
	// Fetch all previous inventories.
	previousInventories, err := a.previousInventoriesFunc(inventoryLabel,
		inventoryObject.Namespace)
	if err != nil {
		return nil, err
	}

	sort.Sort(ResourceInfos(resources))

	if !validateNamespace(resources) {
		return nil, fmt.Errorf("objects have differing namespaces")
	}

	return &ResourceObjects{
		CurrentInventory:    inventoryObject,
		PreviousInventories: previousInventories,
		Resources:           resources,
	}, nil
}

// ResourceObjects contains information about the resources that
// will be applied and the existing inventories used to determine
// resources that should be pruned.
type ResourceObjects struct {
	CurrentInventory    *resource.Info
	PreviousInventories []*resource.Info
	Resources           []*resource.Info
}

// InfosForApply returns the infos representation for all the resources
// that should be applied, including the inventory object. The
// resources will be in sorted order.
func (r *ResourceObjects) InfosForApply() []*resource.Info {
	return append([]*resource.Info{r.CurrentInventory}, r.Resources...)
}

// IdsForApply returns the Ids for all resources that should be applied,
// including the inventory object.
func (r *ResourceObjects) IdsForApply() []object.ObjMetadata {
	var ids []object.ObjMetadata
	for _, info := range r.InfosForApply() {
		ids = append(ids, object.InfoToObjMeta(info))
	}
	return ids
}

// IdsForPrune returns the Ids for all resources that should
// be pruned.
func (r *ResourceObjects) IdsForPrune() []object.ObjMetadata {
	inventory, _ := prune.UnionPastObjs(r.PreviousInventories)

	applyIds := make(map[object.ObjMetadata]bool)
	for _, id := range r.IdsForApply() {
		applyIds[id] = true
	}

	var ids []object.ObjMetadata
	for _, id := range inventory {
		if _, found := applyIds[id]; found {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// AllIds returns the Ids for all resources that are relevant. This
// includes resources that will be applied or pruned.
func (r *ResourceObjects) AllIds() []object.ObjMetadata {
	return append(r.IdsForApply(), r.IdsForPrune()...)
}

// splitInfos takes a slice of resource.Info objects and splits it
// into one slice that contains the inventory object templates and
// another one that contains the remaining resources.
func splitInfos(infos []*resource.Info) ([]*resource.Info, []*resource.Info) {
	inventoryObjectTemplates := make([]*resource.Info, 0)
	resources := make([]*resource.Info, 0)

	for _, info := range infos {
		if prune.IsInventoryObject(info.Object) {
			inventoryObjectTemplates = append(inventoryObjectTemplates, info)
		} else {
			resources = append(resources, info)
		}
	}
	return resources, inventoryObjectTemplates
}

// Run performs the Apply step. This happens asynchronously with updates
// on progress and any errors are reported back on the event channel.
// Cancelling the operation or setting timeout on how long to Wait
// for it complete can be done with the passed in context.
// Note: There sn't currently any way to interrupt the operation
// before all the given resources have been applied to the cluster. Any
// cancellation or timeout will only affect how long we Wait for the
// resources to become current.
func (a *Applier) Run(ctx context.Context, options Options) <-chan event.Event {
	eventChannel := make(chan event.Event)
	setDefaults(&options)

	go func() {
		defer close(eventChannel)
		adapter := &KubectlPrinterAdapter{
			ch: eventChannel,
		}
		// The adapter is used to intercept what is meant to be printing
		// in the ApplyOptions, and instead turn those into events.
		a.ApplyOptions.ToPrinter = adapter.toPrinterFunc()

		// This provides us with a slice of all the objects that will be
		// applied to the cluster. This takes care of ordering resources
		// and handling the inventory object.
		resourceObjects, err := a.readAndPrepareObjects()
		if err != nil {
			handleError(eventChannel, err)
			return
		}

		// Fetch the queue (channel) of tasks that should be executed.
		taskQueue := (&solver.TaskQueueSolver{
			ApplyOptions: a.ApplyOptions,
			PruneOptions: a.PruneOptions,
			InfoHelper:   a.infoHelperFactoryFunc(),
		}).BuildTaskQueue(resourceObjects, solver.Options{
			ReconcileTimeout:       options.ReconcileTimeout,
			Prune:                  !options.NoPrune,
			DryRun:                 options.DryRun,
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

	// DryRun defines whether changes should actually be performed,
	// or if it is just talk and no action.
	DryRun bool

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

// validateNamespace returns true if all the objects in the passed
// infos parameter have the same namespace; false otherwise. Ignores
// cluster-scoped resources.
func validateNamespace(infos []*resource.Info) bool {
	currentNamespace := metav1.NamespaceNone
	for _, info := range infos {
		// Ignore cluster-scoped resources.
		if info.Namespaced() {
			// If the current namespace has not been set--then set it.
			if currentNamespace == metav1.NamespaceNone {
				currentNamespace = info.Namespace
			}
			if currentNamespace != info.Namespace {
				return false
			}
		}
	}
	return true
}

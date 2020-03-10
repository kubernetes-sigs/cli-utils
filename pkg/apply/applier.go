// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
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
	return &Applier{
		ApplyOptions:  apply.NewApplyOptions(ioStreams),
		StatusOptions: NewStatusOptions(),
		PruneOptions:  prune.NewPruneOptions(),
		factory:       factory,
		ioStreams:     ioStreams,
	}
}

// poller defines the interface the applier needs to poll for status of resources.
type poller interface {
	Poll(ctx context.Context, identifiers []object.ObjMetadata, options polling.Options) <-chan pollevent.Event
}

// Applier performs the step of applying a set of resources into a cluster,
// conditionally waits for all of them to be fully reconciled and finally
// performs prune to clean up any resources that has been deleted.
type Applier struct {
	factory   util.Factory
	ioStreams genericclioptions.IOStreams

	ApplyOptions  *apply.ApplyOptions
	StatusOptions *StatusOptions
	PruneOptions  *prune.PruneOptions
	statusPoller  poller

	NoPrune bool
	DryRun  bool
}

// Initialize sets up the Applier for actually doing an apply against
// a cluster. This involves validating command line inputs and configuring
// clients for communicating with the cluster.
func (a *Applier) Initialize(cmd *cobra.Command, paths []string) error {
	fileNameFlags, err := demandOneDirectory(paths)
	if err != nil {
		return err
	}
	a.ApplyOptions.DeleteFlags.FileNameFlags = &fileNameFlags
	err = a.ApplyOptions.Complete(a.factory, cmd)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up ApplyOptions", 1)
	}
	a.ApplyOptions.PreProcessorFn = prune.PrependGroupingObject(a.ApplyOptions)
	a.ApplyOptions.PostProcessorFn = nil // Turn off the default kubectl pruning
	err = a.PruneOptions.Initialize(a.factory)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up PruneOptions", 1)
	}

	// Propagate dry-run flags.
	a.ApplyOptions.DryRun = a.DryRun
	a.PruneOptions.DryRun = a.DryRun

	statusPoller, err := a.newStatusPoller()
	if err != nil {
		return errors.WrapPrefix(err, "error creating resolver", 1)
	}
	a.statusPoller = statusPoller
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
	a.StatusOptions.AddFlags(cmd)
	a.ApplyOptions.Overwrite = true
	return nil
}

// newStatusPoller sets up a new StatusPoller for computing status. The configuration
// needed for the poller is taken from the Factory.
func (a *Applier) newStatusPoller() (poller, error) {
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

// Run performs the Apply step. This happens asynchronously with updates
// on progress and any errors are reported back on the event channel.
// Cancelling the operation or setting timeout on how long to wait
// for it complete can be done with the passed in context.
// Note: There sn't currently any way to interrupt the operation
// before all the given resources have been applied to the cluster. Any
// cancellation or timeout will only affect how long we wait for the
// resources to become current.
func (a *Applier) Run(ctx context.Context) <-chan event.Event {
	ch := make(chan event.Event)

	go func() {
		defer close(ch)
		adapter := &KubectlPrinterAdapter{
			ch: ch,
		}
		// The adapter is used to intercept what is meant to be printing
		// in the ApplyOptions, and instead turn those into events.
		a.ApplyOptions.ToPrinter = adapter.toPrinterFunc()
		// This provides us with a slice of all the objects that will be
		// applied to the cluster.
		infos, err := a.ApplyOptions.GetObjects()
		if err != nil {
			ch <- event.Event{
				Type: event.ErrorType,
				ErrorEvent: event.ErrorEvent{
					Err: errors.WrapPrefix(err, "error reading resource manifests", 1),
				},
			}
			return
		}

		// Validate the objects are in the same namespace.
		if !validateNamespace(infos) {
			err := fmt.Errorf("currently, applied objects must be in the same namespace")
			ch <- event.Event{
				Type: event.ErrorType,
				ErrorEvent: event.ErrorEvent{
					Err: errors.WrapPrefix(err, "objects have differing namespaces", 1),
				},
			}
			return
		}

		// sort the info objects starting from independent to dependent objects,
		// and set them back ordering precedence can be found in resource_infos.go
		sort.Sort(ResourceInfos(infos))
		a.ApplyOptions.SetObjects(infos)

		err = a.ApplyOptions.Run()
		if err != nil {
			// If we see an error here we just report it on the channel and then
			// give up. Eventually we might be able to determine which errors
			// are fatal and which might allow us to continue.
			ch <- event.Event{
				Type: event.ErrorType,
				ErrorEvent: event.ErrorEvent{
					Err: errors.WrapPrefix(err, "error applying resources", 1),
				},
			}
			return
		}
		// If we get there, then all resources have been successfully applied.
		ch <- event.Event{
			Type: event.ApplyType,
			ApplyEvent: event.ApplyEvent{
				Type: event.ApplyEventCompleted,
			},
		}

		if a.StatusOptions.wait {
			statusChannel := a.statusPoller.Poll(ctx, infosToObjMetas(infos), polling.Options{
				PollUntilCancelled: true,
				PollInterval:       a.StatusOptions.period,
				UseCache:           true,
				DesiredStatus:      status.CurrentStatus,
			})
			// As long as the statusChannel remains open, we take every statusEvent,
			// wrap it in an Event and send it on the channel.
			// TODO: What should we do if waiting for status times out? We currently proceed with
			// prune, but that doesn't seem right.
			for statusEvent := range statusChannel {
				ch <- event.Event{
					Type:        event.StatusType,
					StatusEvent: statusEvent,
				}
			}
		}

		if !a.NoPrune {
			err = a.PruneOptions.Prune(infos, ch)
			if err != nil {
				// If we see an error here we just report it on the channel and then
				// give up. Eventually we might be able to determine which errors
				// are fatal and which might allow us to continue.
				ch <- event.Event{
					Type: event.ErrorType,
					ErrorEvent: event.ErrorEvent{
						Err: errors.WrapPrefix(err, "error pruning resources", 1),
					},
				}
				return
			}
			ch <- event.Event{
				Type: event.PruneType,
				PruneEvent: event.PruneEvent{
					Type: event.PruneEventCompleted,
				},
			}
		}
	}()
	return ch
}

func infosToObjMetas(infos []*resource.Info) []object.ObjMetadata {
	var objMetas []object.ObjMetadata
	for _, info := range infos {
		u := info.Object.(*unstructured.Unstructured)
		objMetas = append(objMetas, object.ObjMetadata{
			GroupKind: u.GroupVersionKind().GroupKind(),
			Name:      u.GetName(),
			Namespace: u.GetNamespace(),
		})
	}
	return objMetas
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

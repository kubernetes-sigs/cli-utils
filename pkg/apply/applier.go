// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"time"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
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
		factory:       factory,
		ioStreams:     ioStreams,
	}
}

// resolver defines the interface the applier needs to observe status for resources.
type resolver interface {
	WaitForStatusOfObjects(ctx context.Context, objects []wait.KubernetesObject) <-chan wait.Event
}

// Applier performs the step of applying a set of resources into a cluster,
// conditionally waits for all of them to be fully reconciled and finally
// performs prune to clean up any resources that has been deleted.
type Applier struct {
	factory   util.Factory
	ioStreams genericclioptions.IOStreams

	ApplyOptions  *apply.ApplyOptions
	StatusOptions *StatusOptions
	resolver      resolver
}

// Initialize sets up the Applier for actually doing an apply against
// a cluster. This involves validating command line inputs and configuring
// clients for communicating with the cluster.
func (a *Applier) Initialize(cmd *cobra.Command) error {
	a.ApplyOptions.PreProcessorFn = prependGroupingObject(a.ApplyOptions)
	err := a.ApplyOptions.Complete(a.factory, cmd)
	if err != nil {
		return errors.WrapPrefix(err, "error setting up ApplyOptions", 1)
	}
	// Default PostProcessor is configured in "Complete" function,
	// so the prune must happen after "Complete".
	a.ApplyOptions.PostProcessorFn = pruneExec(a.factory, a.ApplyOptions)

	resolver, err := a.newResolver(a.StatusOptions.period)
	if err != nil {
		return errors.WrapPrefix(err, "error creating resolver", 1)
	}
	a.resolver = resolver
	return nil
}

// SetFlags configures the command line flags needed for apply and
// status. This is a temporary solution as we should separate the configuration
// of cobra flags from the Applier.
func (a *Applier) SetFlags(cmd *cobra.Command) {
	a.ApplyOptions.DeleteFlags.AddFlags(cmd)
	a.ApplyOptions.RecordFlags.AddFlags(cmd)
	a.ApplyOptions.PrintFlags.AddFlags(cmd)
	a.StatusOptions.AddFlags(cmd)
	a.ApplyOptions.Overwrite = true
}

// newResolver sets up a new Resolver for computing status. The configuration
// needed for the resolver is taken from the Factory.
func (a *Applier) newResolver(pollInterval time.Duration) (*wait.Resolver, error) {
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

	return wait.NewResolver(c, mapper, pollInterval), nil
}

// Run performs the Apply step. This happens asynchronously with updates
// on progress and any errors are reported back on the event channel.
// Cancelling the operation or setting timeout on how long to wait
// for it complete can be done with the passed in context.
// Note: There sn't currently any way to interrupt the operation
// before all the given resources have been applied to the cluster. Any
// cancellation or timeout will only affect how long we wait for the
// resources to become current.
func (a *Applier) Run(ctx context.Context) <-chan Event {
	ch := make(chan Event)

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
		infos, _ := a.ApplyOptions.GetObjects()
		err := a.ApplyOptions.Run()
		if err != nil {
			// If we see an error here we just report it on the channel and then
			// give up. Eventually we might be able to determine which errors
			// are fatal and which might allow us to continue.
			ch <- Event{
				EventType: ErrorEventType,
				ErrorEvent: ErrorEvent{
					Err: errors.WrapPrefix(err, "error applying resources", 1),
				},
			}
			return
		}

		if a.StatusOptions.wait {
			statusChannel := a.resolver.WaitForStatusOfObjects(ctx, infosToObjects(infos))
			// As long as the statusChannel remains open, we take every statusEvent,
			// wrap it in an Event and send it on the channel.
			for statusEvent := range statusChannel {
				ch <- Event{
					EventType:   StatusEventType,
					StatusEvent: statusEvent,
				}
			}
		}

	}()
	return ch
}

func infosToObjects(infos []*resource.Info) []wait.KubernetesObject {
	var objects []wait.KubernetesObject
	for _, info := range infos {
		u := info.Object.(*unstructured.Unstructured)
		objects = append(objects, u)
	}
	return objects
}

// EventType determines the type of events that are available.
type EventType string

const (
	ErrorEventType  EventType = "error"
	ApplyEventType  EventType = "apply"
	StatusEventType EventType = "status"
)

// Event is the type of the objects that will be returned through
// the channel that is returned from a call to Run. It contains
// information about progress and errors encountered during
// the process of doing apply, waiting for status and doing a prune.
type Event struct {
	// EventType is the type of event.
	EventType EventType

	// ErrorEvent contains information about any errors encountered.
	ErrorEvent ErrorEvent

	// ApplyEvent contains information about progress pertaining to
	// applying a resource to the cluster.
	ApplyEvent ApplyEvent

	// StatusEvents contains information about the status of one of
	// the applied resources.
	StatusEvent wait.Event
}

type ErrorEvent struct {
	Err error
}

type ApplyEvent struct {
	Operation string
	Object    runtime.Object
}

// PrependGroupingObject orders the objects to apply so the "grouping"
// object stores the inventory, and it is first to be applied.
func prependGroupingObject(o *apply.ApplyOptions) func() error {
	return func() error {
		if o == nil {
			return fmt.Errorf("ApplyOptions are nil")
		}
		infos, err := o.GetObjects()
		if err != nil {
			return err
		}
		_, exists := prune.FindGroupingObject(infos)
		if exists {
			if err := prune.AddInventoryToGroupingObj(infos); err != nil {
				return err
			}
			if !prune.SortGroupingObject(infos) {
				return err
			}
		}
		return nil
	}
}

// Prune deletes previously applied objects that have been
// omitted in the current apply. The previously applied objects
// are reached through ConfigMap grouping objects.
func pruneExec(f util.Factory, o *apply.ApplyOptions) func() error {
	return func() error {
		po, err := prune.NewPruneOptions(f, o)
		if err != nil {
			return err
		}
		return po.Prune()
	}
}

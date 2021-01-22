// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"fmt"

	"github.com/go-errors/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/provider"
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
	provider       provider.Provider
	PruneOptions   *prune.PruneOptions
	invClient      inventory.InventoryClient
	DryRunStrategy common.DryRunStrategy
}

type DestroyerOption struct {
	InventoryPolicy inventory.InventoryPolicy
}

// Initialize sets up the Destroyer for actually doing an destroy against
// a cluster. This involves validating command line inputs and configuring
// clients for communicating with the cluster.
func (d *Destroyer) Initialize() error {
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
	ch := make(chan event.Event)

	go func() {
		defer close(ch)
		d.invClient.SetDryRunStrategy(d.DryRunStrategy)

		// Start the event transformer goroutine so we can transform
		// the Prune events emitted from the Prune function to Delete
		// Events. That we use Prune to implement destroy is an
		// implementation detail and the events should not be Prune events.
		tempChannel, completedChannel := runPruneEventTransformer(ch)
		taskContext := taskrunner.NewTaskContext(tempChannel)
		err := d.PruneOptions.Prune(inv, nil, sets.NewString(), taskContext, prune.Options{
			DryRunStrategy:    d.DryRunStrategy,
			PropagationPolicy: metav1.DeletePropagationBackground,
			InventoryPolicy:   option.InventoryPolicy,
		})
		if err != nil {
			ch <- event.Event{
				Type: event.ErrorType,
				ErrorEvent: event.ErrorEvent{
					Err: errors.WrapPrefix(err, "error pruning resources in cluster", 1),
				},
			}
			return
		}

		// Now delete the inventory object as well.
		err = d.invClient.DeleteInventoryObj(inv)
		if err != nil {
			ch <- event.Event{
				Type: event.ErrorType,
				ErrorEvent: event.ErrorEvent{
					Err: errors.WrapPrefix(err, "error deleting inventory object", 1),
				},
			}
			return
		}

		// Close the tempChannel to signal to the event transformer that
		// it should terminate.
		close(tempChannel)
		// Wait for the event transformer to complete processing all
		// events and shut down before we continue.
		<-completedChannel
		ch <- event.Event{
			Type: event.DeleteType,
			DeleteEvent: event.DeleteEvent{
				Type: event.DeleteEventCompleted,
			},
		}
	}()
	return ch
}

// runPruneEventTransformer creates a channel for events and
// starts a goroutine that will read from the channel until it
// is closed. All events will be republished as Delete events
// on the provided eventChannel. The function will also return
// a channel that it will close once the goroutine is shutting
// down.
func runPruneEventTransformer(eventChannel chan event.Event) (chan event.Event, <-chan struct{}) {
	completedChannel := make(chan struct{})
	tempEventChannel := make(chan event.Event)
	go func() {
		defer close(completedChannel)
		for msg := range tempEventChannel {
			if msg.PruneEvent.Type == event.PruneEventFailed {
				eventChannel <- event.Event{
					Type: event.DeleteType,
					DeleteEvent: event.DeleteEvent{
						Type:       event.DeleteEventFailed,
						Operation:  transformPruneOperation(msg.PruneEvent.Operation),
						Object:     msg.PruneEvent.Object,
						Identifier: msg.PruneEvent.Identifier,
					},
				}
				continue
			}
			eventChannel <- event.Event{
				Type: event.DeleteType,
				DeleteEvent: event.DeleteEvent{
					Type:       event.DeleteEventResourceUpdate,
					Operation:  transformPruneOperation(msg.PruneEvent.Operation),
					Object:     msg.PruneEvent.Object,
					Identifier: msg.PruneEvent.Identifier,
				},
			}
		}
	}()
	return tempEventChannel, completedChannel
}

func transformPruneOperation(pruneOp event.PruneEventOperation) event.DeleteEventOperation {
	switch pruneOp {
	case event.PruneSkipped:
		return event.DeleteSkipped
	case event.Pruned:
		return event.Deleted
	default:
		panic(fmt.Errorf("unknown prune operation %s", pruneOp.String()))
	}
}

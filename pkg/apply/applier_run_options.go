// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
)

const defaultPollInterval = 2 * time.Second

type applierRunConfig struct {
	// Encapsulates the fields for server-side apply.
	serverSideOptions common.ServerSideOptions

	// reconcileTimeout defines whether the applier should wait
	// until all applied resources have been reconciled, and if so,
	// how long to wait.
	reconcileTimeout time.Duration

	// pollInterval defines how often we should poll for the status
	// of resources.
	pollInterval time.Duration

	// emitStatusEvents defines whether status events should be
	// emitted on the eventChannel to the caller.
	emitStatusEvents bool

	// prune defines whether pruning of previously applied
	// objects should happen after apply.
	prune bool

	// dryRunStrategy defines whether changes should actually be performed,
	// or if it is just talk and no action.
	dryRunStrategy common.DryRunStrategy

	// prunePropagationPolicy defines the deletion propagation policy
	// that should be used for pruning. If this is not provided, the
	// default is to use the Background policy.
	prunePropagationPolicy metav1.DeletionPropagation

	// pruneTimeout defines whether we should wait for all resources
	// to be fully deleted after pruning, and if so, how long we should
	// wait.
	pruneTimeout time.Duration

	// inventoryPolicy defines the inventory policy of apply.
	inventoryPolicy inventory.InventoryPolicy

	// eventListeners is a list of event listeners that are called in specified order when an event happens.
	// Listeners are never called concurrently with each other.
	eventListeners []func(event.Event)
}

type ApplierRunOption func(*applierRunConfig)

// constructApplierRunConfig turns options into a config.
// It always returns a non-nil config, event if there was an error.
func constructApplierRunConfig(opts []ApplierRunOption) *applierRunConfig {
	cfg := defaultApplierRunConfig()
	setOptsOnApplierRunConfig(cfg, opts)
	return cfg
}

func defaultApplierRunConfig() *applierRunConfig {
	return &applierRunConfig{
		pollInterval:           defaultPollInterval,
		prune:                  true,
		prunePropagationPolicy: metav1.DeletePropagationBackground,
	}
}

func setOptsOnApplierRunConfig(cfg *applierRunConfig, opts []ApplierRunOption) {
	for _, opt := range opts {
		opt(cfg)
	}
}

func ServerSideOptions(serverSideOptions common.ServerSideOptions) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.serverSideOptions = serverSideOptions
	}
}

func ReconcileTimeout(reconcileTimeout time.Duration) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.reconcileTimeout = reconcileTimeout
	}
}

func PollInterval(pollInterval time.Duration) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.pollInterval = pollInterval
	}
}

func EmitStatusEvents(emitStatusEvents bool) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.emitStatusEvents = emitStatusEvents
	}
}

func Prune(prune bool) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.prune = prune
	}
}

func DryRunStrategy(dryRunStrategy common.DryRunStrategy) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.dryRunStrategy = dryRunStrategy
	}
}

func PrunePropagationPolicy(prunePropagationPolicy metav1.DeletionPropagation) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.prunePropagationPolicy = prunePropagationPolicy
	}
}

func PruneTimeout(pruneTimeout time.Duration) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.pruneTimeout = pruneTimeout
	}
}

func InventoryPolicy(inventoryPolicy inventory.InventoryPolicy) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.inventoryPolicy = inventoryPolicy
	}
}

func EventListener(eventListener ...func(event.Event)) ApplierRunOption {
	return func(cfg *applierRunConfig) {
		cfg.eventListeners = append(cfg.eventListeners, eventListener...)
	}
}

func EventChannelListener(eventChannel chan<- event.Event) ApplierRunOption {
	return EventListener(func(e event.Event) {
		eventChannel <- e
	})
}

func CollectEventsInto(eventSink *[]event.Event) ApplierRunOption {
	return EventListener(func(e event.Event) {
		*eventSink = append(*eventSink, e)
	})
}

// CollectErrorInto captures error from the first event of ErrorType type.
func CollectErrorInto(errSink *error) ApplierRunOption {
	assigned := false
	return EventListener(func(e event.Event) {
		if e.Type == event.ErrorType && !assigned {
			assigned = true
			*errSink = e.ErrorEvent.Err
		}
	})
}

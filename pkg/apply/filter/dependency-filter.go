// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// DependencyFilter implements ValidationFilter interface to determine if an
// object can be applied or deleted based on the status of it's dependencies.
type DependencyFilter struct {
	TaskContext *taskrunner.TaskContext
	Strategy    actuation.ActuationStrategy
}

const DependencyFilterName = "DependencyFilter"

// Name returns the name of the filter for logs and events.
func (dnrf DependencyFilter) Name() string {
	return DependencyFilterName
}

// Filter returns true if the specified object should be skipped because at
// least one of its dependencies is Not Found or Not Reconciled.
func (dnrf DependencyFilter) Filter(obj *unstructured.Unstructured) (bool, string, error) {
	id := object.UnstructuredToObjMetadata(obj)

	switch dnrf.Strategy {
	case actuation.ActuationStrategyApply:
		// For apply, check dependencies
		relationship := "dependency"
		for _, depID := range dnrf.TaskContext.Graph.EdgesFrom(id) {
			filter, reason, err := dnrf.filterByRelationStatus(depID, relationship)
			if err != nil {
				return false, "", err
			}
			if filter {
				return filter, reason, nil
			}
		}
	case actuation.ActuationStrategyDelete:
		// For delete, check dependents (aka "incoming dependencies")
		relationship := "dependent"
		for _, depID := range dnrf.TaskContext.Graph.EdgesTo(id) {
			filter, reason, err := dnrf.filterByRelationStatus(depID, relationship)
			if err != nil {
				return false, "", err
			}
			if filter {
				return filter, reason, nil
			}
		}
	default:
		panic(fmt.Sprintf("invalid filter strategy: %q", dnrf.Strategy))
	}
	return false, "", nil
}

func (dnrf DependencyFilter) filterByRelationStatus(id object.ObjMetadata, relationship string) (bool, string, error) {
	// skip!
	if dnrf.TaskContext.IsInvalidObject(id) {
		return true, fmt.Sprintf("%s invalid: %q", relationship, id), nil
	}

	status, found := dnrf.TaskContext.InventoryManager().ObjectStatus(id)
	if !found {
		// status is registered during planning, so if not found, the object is external (NYI)
		return false, "", fmt.Errorf("unknown %s actuation strategy: %v", relationship, id)
	}

	// dependencies must have the same actuation strategy
	if status.Strategy != dnrf.Strategy {
		return true, fmt.Sprintf("%s actuation strategy mismatch (%q != %q): %q", relationship, dnrf.Strategy, status.Strategy, id), nil
	}

	switch status.Actuation {
	case actuation.ActuationPending:
		// If actuation is still pending, dependency sorting is probably broken
		return false, "", fmt.Errorf("premature actuation: %s %s %s: %q", relationship, dnrf.Strategy, status.Actuation, id)
	case actuation.ActuationSkipped, actuation.ActuationFailed:
		// skip!
		return true, fmt.Sprintf("%s %s %s: %q", relationship, dnrf.Strategy, status.Actuation, id), nil
	case actuation.ActuationSucceeded:
		// yay!
	default:
		return false, "", fmt.Errorf("invalid %s apply status %q: %q", relationship, status.Actuation, id)
	}

	switch status.Reconcile {
	case actuation.ReconcilePending:
		// If reconcile is still pending, dependency sorting is probably broken
		return false, "", fmt.Errorf("premature reconciliation: %s reconcile %s: %q", relationship, status.Reconcile, id)
	case actuation.ReconcileSkipped, actuation.ReconcileFailed, actuation.ReconcileTimeout:
		// skip!
		return true, fmt.Sprintf("%s reconcile %s: %q", relationship, status.Reconcile, id), nil
	case actuation.ReconcileSucceeded:
		// yay!
	default:
		return false, "", fmt.Errorf("invalid dependency reconcile status %q: %q", status.Reconcile, id)
	}

	return false, "", nil
}

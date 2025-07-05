// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// isObjectUnschedulable returns true if the object or any of its generated resources
// is an unschedulable pod.
//
// This status is computed recursively, so it can handle objects that generate
// objects that generate pods, as long as the input ResourceStatus has those
// GeneratedResources computed.
func isObjectUnschedulable(rs *event.ResourceStatus) bool {
	if rs.Error != nil {
		return false
	}
	if rs.Status != status.InProgressStatus {
		return false
	}
	if isPodUnschedulable(rs.Resource) {
		return true
	}
	// recurse through generated resources
	return slices.ContainsFunc(rs.GeneratedResources, isObjectUnschedulable)
}

// isPodUnschedulable returns true if the object is a pod and is unschedulable
// according to a False PodScheduled condition.
func isPodUnschedulable(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	gk := obj.GroupVersionKind().GroupKind()
	if gk != (schema.GroupKind{Kind: "Pod"}) {
		return false
	}
	icnds, found, err := object.NestedField(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	cnds, ok := icnds.([]any)
	if !ok {
		return false
	}
	for _, icnd := range cnds {
		cnd, ok := icnd.(map[string]any)
		if !ok {
			return false
		}
		if cnd["type"] == "PodScheduled" &&
			cnd["status"] == "False" &&
			cnd["reason"] == "Unschedulable" {
			return true
		}
	}
	return false
}

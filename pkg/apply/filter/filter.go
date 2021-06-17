// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ValidationFilter interface decouples apply/prune validation
// from the concrete structs used for validation. The apply/prune
// functionality will run validation filters to remove objects
// which should not be applied or pruned.
type ValidationFilter interface {
	// Name returns a filter name (usually for logging).
	Name() string
	// Filter returns true if validation fails or an error.
	Filter(obj *unstructured.Unstructured) (bool, error)
}

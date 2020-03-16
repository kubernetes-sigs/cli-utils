// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Grouping object errors.

package prune

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/resource"
)

const noGroupingErrorStr = `Package uninitialized. Please run "init" command.

The package needs to be initialized to generate the template
which will store state for grouped resources. This state is
necessary to perform functionality such as deleting an entire
package or automatically deleting omitted resources (pruning).
`

const multipleGroupingErrorStr = `Package has multiple grouping object templates.

The package should have one and only one grouping object template.
`

var groupingObjNamespaceError = `Attempting to apply namespace at the same time as the applied resources.

Namespace: %s

Currently, the namespace that applied resources are to be inserted into must exist before applying.
`

type NoGroupingObjError struct{}

func (g NoGroupingObjError) Error() string {
	return noGroupingErrorStr
}

type MultipleGroupingObjError struct {
	GroupingObjectTemplates []*resource.Info
}

func (g MultipleGroupingObjError) Error() string {
	return multipleGroupingErrorStr
}

// GroupingObjNamespaceError encapsulates error where the namespace
// for the applied objects is being applied at the same time as the
// applied objects, including the grouping object. This currently
// fails because the algorithm attempts to apply the grouping object
// first which will fail if the namespace the ConfigMap is supposed
// to be in does not yet exist.
type GroupingObjNamespaceError struct {
	Namespace string
}

func (g GroupingObjNamespaceError) Error() string {
	return fmt.Sprintf(groupingObjNamespaceError, g.Namespace)
}

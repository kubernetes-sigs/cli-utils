// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// Errors when applying inventory object templates.

package prune

import "k8s.io/cli-runtime/pkg/resource"

const noInventoryErrorStr = `Package uninitialized. Please run "init" command.

The package needs to be initialized to generate the template
which will store state for resource sets. This state is
necessary to perform functionality such as deleting an entire
package or automatically deleting omitted resources (pruning).
`

const multipleInventoryErrorStr = `Package has multiple inventory object templates.

The package should have one and only one inventory object template.
`

type NoInventoryObjError struct{}

func (g NoInventoryObjError) Error() string {
	return noInventoryErrorStr
}

type MultipleInventoryObjError struct {
	InventoryObjectTemplates []*resource.Info
}

func (g MultipleInventoryObjError) Error() string {
	return multipleInventoryErrorStr
}

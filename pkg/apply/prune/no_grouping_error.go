// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// The error returned when applying resources, but not
// finding the required grouping object template.

package prune

const noGroupingErrorStr = `Package uninitialized. Please run "init" command.

The package needs to be initialized to generate the template
which will store state for grouped resources. This state is
necessary to perform functionality such as deleting an entire
package or automatically deleting omitted resources (pruning).
`

type NoGroupingObjError struct{}

func (g NoGroupingObjError) Error() string {
	return noGroupingErrorStr
}

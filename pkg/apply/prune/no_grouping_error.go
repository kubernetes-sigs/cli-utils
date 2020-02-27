// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0
//
// The error returned when applying resources, but not
// finding the required grouping object template.

package prune

const noGroupingErrorStr = `

The grouping object template was not found while applying. kpt
live commands require the grouping object template to define
the set of "grouped" objects. An example of a grouping object
template is:

apiVersion: v1
kind: ConfigMap
metadata:
  name: grouping
  labels:
    "cli-utils.sigs.k8s.io/inventory-id": "my-app"

The two requirements for a grouping object template are:

1. It must be a ConfigMap
2. It must contain the "grouping" label:

  cli-utils.sigs.k8s.io/inventory-id: <GROUP-NAME>

When the grouping object template is applied, a specific
grouping object is created, storing the inventory of object
metadata of all objects applied.
`

type NoGroupingObjError struct{}

func (g NoGroupingObjError) Error() string {
	return noGroupingErrorStr
}

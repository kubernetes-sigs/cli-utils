// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package kstatus contains libraries for computing status of kubernetes
// resources.
//
// status
// Get status and/or conditions for resources based on resources already
// read from a cluster, i.e. it will not fetch resources from
// a cluster.
//
// polling
// Poll the cluster for the state of the specified resources
// and compute the status for each as well as the aggregate
// status. The polling will go on until either all resources
// have reached the desired status or the polling is cancelled
// by the caller.
// A common use case for this would to be poll until all resources
// finish reconciling after apply.
package kstatus

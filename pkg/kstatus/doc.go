// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package kstatus contains libraries for computing status of Kubernetes
// resource objects.
//
// status
// Compute the status of Kubernetes resource objects.
//
// polling
// Poll the cluster for the state of the specified resource objects and compute
// the status for each as well as the aggregate status. The polling will
// continue until either all resources have reached the desired status or the
// polling is cancelled by the caller.
//
// watcher
// Watch the cluster for the state of the specified resource objects and compute
// the status for each. The watching will continue until cancelled by the
// caller.
//
// A common use case for this would to be poll/watch until all resource objects
// finish reconciling after apply.
package kstatus

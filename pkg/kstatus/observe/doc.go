// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package observe is a library for computing the status of kubernetes resources
// based on polling of resource state from a cluster. It can keep polling until
// either some condition is met, or until it is cancelled through the provided
// context. Updates on the status of resources are streamed back to the caller
// through a channel.
//
// This package provides a simple interface based on the built-in
// features. But the actual code is in the subpackages and there
// are several interfaces that can be implemented to support custom
// behavior.
//
// Note that while the observe package and its subpackages often use the
// word Observer in both interfaces and objects, this does NOT refer to
// the Observer design pattern. Rather it comes from k8s API conventions and
// refer to the process of checking the state of resources.
//
// Observing Resources
//
// In order to observe a set of resources, create a StatusObserver
// and pass in the list of ResourceIdentifiers to the Observe function.
//
//   import (
//     "sigs.k8s.io/cli-utils/pkg/kstatus/observe"
//   )
//
//   identifiers := []wait.ResourceIdentifier{
//     {
//       GroupKind: schema.GroupKind{
//         Group: "apps",
//         Kind: "Deployment",
//       },
//       Name: "dep",
//       Namespace: "default",
//     }
//   }
//
//   observer := observe.NewStatusObserver(reader, mapper, true)
//   eventsChan := observer.Observe(context.Background(), identifiers, observe.Options{})
//   for e := range eventsChan {
//      // Handle event
//   }
package observe

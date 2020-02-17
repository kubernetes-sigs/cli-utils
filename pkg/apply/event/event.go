// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

// Type determines the type of events that are available.
//go:generate stringer -type=Type
type Type int

const (
	ErrorType Type = iota
	ApplyType
	StatusType
	PruneType
)

// Event is the type of the objects that will be returned through
// the channel that is returned from a call to Run. It contains
// information about progress and errors encountered during
// the process of doing apply, waiting for status and doing a prune.
type Event struct {
	// Type is the type of event.
	Type Type

	// ErrorEvent contains information about any errors encountered.
	ErrorEvent ErrorEvent

	// ApplyEvent contains information about progress pertaining to
	// applying a resource to the cluster.
	ApplyEvent ApplyEvent

	// StatusEvents contains information about the status of one of
	// the applied resources.
	StatusEvent wait.Event

	// PruneEvent contains information about objects that have been
	// pruned.
	PruneEvent PruneEvent
}

type ErrorEvent struct {
	Err error
}

//go:generate stringer -type=ApplyEventType
type ApplyEventType int

const (
	ApplyEventResourceUpdate ApplyEventType = iota
	ApplyEventCompleted
)

//go:generate stringer -type=ApplyEventOperation
type ApplyEventOperation int

const (
	ServersideApplied ApplyEventOperation = iota
	Created
	Unchanged
	Configured
)

type ApplyEvent struct {
	Type      ApplyEventType
	Operation ApplyEventOperation
	Object    runtime.Object
}

//go:generate stringer -type=PruneEventType
type PruneEventType int

const (
	PruneEventResourceUpdate PruneEventType = iota
	PruneEventCompleted
)

type PruneEvent struct {
	Type   PruneEventType
	Object runtime.Object
}

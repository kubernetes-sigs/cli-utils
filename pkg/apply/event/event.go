// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Type determines the type of events that are available.
//go:generate stringer -type=Type
type Type int

const (
	InitType Type = iota
	ErrorType
	ApplyType
	StatusType
	PruneType
	DeleteType
)

// Event is the type of the objects that will be returned through
// the channel that is returned from a call to Run. It contains
// information about progress and errors encountered during
// the process of doing apply, waiting for status and doing a prune.
type Event struct {
	// Type is the type of event.
	Type Type

	// InitEvent contains information about which resources will
	// be applied/pruned.
	InitEvent InitEvent

	// ErrorEvent contains information about any errors encountered.
	ErrorEvent ErrorEvent

	// ApplyEvent contains information about progress pertaining to
	// applying a resource to the cluster.
	ApplyEvent ApplyEvent

	// StatusEvents contains information about the status of one of
	// the applied resources.
	StatusEvent StatusEvent

	// PruneEvent contains information about objects that have been
	// pruned.
	PruneEvent PruneEvent

	// DeleteEvent contains information about object that have been
	// deleted.
	DeleteEvent DeleteEvent
}

type InitEvent struct {
	ResourceGroups []ResourceGroup
}

//go:generate stringer -type=ResourceAction
type ResourceAction int

const (
	ApplyAction ResourceAction = iota
	PruneAction
)

type ResourceGroup struct {
	Action      ResourceAction
	Identifiers []object.ObjMetadata
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
	Failed
)

type ApplyEvent struct {
	Type       ApplyEventType
	Operation  ApplyEventOperation
	Object     *unstructured.Unstructured
	Identifier object.ObjMetadata
	Error      error
}

//go:generate stringer -type=StatusEventType
type StatusEventType int

const (
	StatusEventResourceUpdate StatusEventType = iota
	StatusEventCompleted
)

type StatusEvent struct {
	Type     StatusEventType
	Resource *pollevent.ResourceStatus
}

//go:generate stringer -type=PruneEventType
type PruneEventType int

const (
	PruneEventResourceUpdate PruneEventType = iota
	PruneEventCompleted
	PruneEventFailed
)

//go:generate stringer -type=PruneEventOperation
type PruneEventOperation int

const (
	Pruned PruneEventOperation = iota
	PruneSkipped
)

type PruneEvent struct {
	Type       PruneEventType
	Operation  PruneEventOperation
	Object     *unstructured.Unstructured
	Identifier object.ObjMetadata
	Error      error
}

//go:generate stringer -type=DeleteEventType
type DeleteEventType int

const (
	DeleteEventResourceUpdate DeleteEventType = iota
	DeleteEventCompleted
	DeleteEventFailed
)

//go:generate stringer -type=DeleteEventOperation
type DeleteEventOperation int

const (
	Deleted DeleteEventOperation = iota
	DeleteSkipped
)

type DeleteEvent struct {
	Type       DeleteEventType
	Operation  DeleteEventOperation
	Object     *unstructured.Unstructured
	Identifier object.ObjMetadata
	Error      error
}

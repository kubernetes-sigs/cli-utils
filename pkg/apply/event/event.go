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
	ActionGroupType
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

	// ActionGroupEvent contains information about the progression of tasks
	// to apply, prune, and destroy resources, and tasks that involves waiting
	// for a set of resources to reach a specific state.
	ActionGroupEvent ActionGroupEvent

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
	ActionGroups []ActionGroup
}

//go:generate stringer -type=ResourceAction
type ResourceAction int

const (
	ApplyAction ResourceAction = iota
	PruneAction
	DeleteAction
	WaitAction
	InventoryAction
)

type ActionGroup struct {
	Name        string
	Action      ResourceAction
	Identifiers object.ObjMetadataSet
}

type ErrorEvent struct {
	Err error
}

//go:generate stringer -type=ActionGroupEventType
type ActionGroupEventType int

const (
	Started ActionGroupEventType = iota
	Finished
)

type ActionGroupEvent struct {
	GroupName string
	Action    ResourceAction
	Type      ActionGroupEventType
}

//go:generate stringer -type=ApplyEventOperation
type ApplyEventOperation int

const (
	ApplyUnspecified ApplyEventOperation = iota
	ServersideApplied
	Created
	Unchanged
	Configured
)

type ApplyEvent struct {
	GroupName  string
	Identifier object.ObjMetadata
	Operation  ApplyEventOperation
	Resource   *unstructured.Unstructured
	Error      error
}

type StatusEvent struct {
	Identifier       object.ObjMetadata
	PollResourceInfo *pollevent.ResourceStatus
	Resource         *unstructured.Unstructured
	Error            error
}

//go:generate stringer -type=PruneEventOperation
type PruneEventOperation int

const (
	PruneUnspecified PruneEventOperation = iota
	Pruned
	PruneSkipped
)

type PruneEvent struct {
	GroupName  string
	Identifier object.ObjMetadata
	Operation  PruneEventOperation
	Object     *unstructured.Unstructured
	// If prune is skipped, this reason string explains why
	Reason string
	Error  error
}

//go:generate stringer -type=DeleteEventOperation
type DeleteEventOperation int

const (
	DeleteUnspecified DeleteEventOperation = iota
	Deleted
	DeleteSkipped
)

type DeleteEvent struct {
	GroupName  string
	Identifier object.ObjMetadata
	Operation  DeleteEventOperation
	Object     *unstructured.Unstructured
	// If delete is skipped, this reason string explains why
	Reason string
	Error  error
}

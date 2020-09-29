package event

import (
	"k8s.io/apimachinery/pkg/runtime"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Event describes something that has happened during the process or either
// applying resources or removing resources from the cluster.
// The Type property defines which event has taken place and the
// corresponding property will contain more information about the details.
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

// Type defines the go type used to enumerate the possible types of events.
type Type int

const (
	InitType Type = iota
	ErrorType
	ApplyType
	StatusType
	PruneType
	DeleteType
)

// InitEvent describes the resources that will be applied, pruned or deleted
// as part of either an apply or destroy operation.
type InitEvent struct {
	ActionGroups []ActionGroup
}

// ResourceAction defines the go type used to enumerate the different actions
// that can be described in the Init event.
type ResourceAction int

const (
	ApplyAction ResourceAction = iota
	PruneAction
	DeleteAction
)

// ActionGroup defines an action for a group of resources. Resources are
// represented with their identifiers (group, kind, name and namespace).
type ActionGroup struct {
	Action      ResourceAction
	Identifiers []object.ObjMetadata
}

// ErrorEvent describes a fatal error that has happened. The operation
// will be aborted.
type ErrorEvent struct {
	Err error
}

// ApplyEventType defines the go type used to enumerate the possible variants
// of the ApplyEvent.
type ApplyEventType int

const (
	ApplyEventResourceUpdate ApplyEventType = iota
	ApplyEventCompleted
)


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

type StatusEventType int

const (
	StatusEventResourceUpdate StatusEventType = iota
	StatusEventError
)

type StatusEvent struct {
	Type StatusEventType
	Resource *pollevent.ResourceStatus
	Error error
}

type PruneEventType int

const (
	PruneEventResourceUpdate PruneEventType = iota
	PruneEventCompleted
)

type PruneEventOperation int

const (
	Pruned PruneEventOperation = iota
	PruneSkipped
)

type PruneEvent struct {
	Type      PruneEventType
	Operation PruneEventOperation
	Object    runtime.Object
}

type DeleteEventType int

const (
	DeleteEventResourceUpdate DeleteEventType = iota
	DeleteEventCompleted
)

type DeleteEventOperation int

const (
	Deleted DeleteEventOperation = iota
	DeleteSkipped
)

type DeleteEvent struct {
	Type      DeleteEventType
	Operation DeleteEventOperation
	Object    runtime.Object
}

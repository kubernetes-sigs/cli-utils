package event

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

// Type determines the type of events that are available.
type Type string

const (
	ErrorEventType  Type = "error"
	ApplyEventType  Type = "apply"
	StatusEventType Type = "status"
	PruneEventType  Type = "prune"
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

type ApplyEvent struct {
	Operation string
	Object    runtime.Object
}

type PruneEvent struct {
	Object runtime.Object
}

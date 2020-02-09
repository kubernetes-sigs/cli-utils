package event

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/kstatus/wait"
)

// EventType is the type that describes the type of an Event that is passed back to the caller
// as resources in the cluster are being observed.
type EventType string

const (
	// ResourceUpdateEvent describes events related to a change in the status of one of the observed resources.
	ResourceUpdateEvent EventType = "ResourceUpdated"
	// CompletedEvent signals that all resources have been reconciled and the observer has completed its work. The
	// event channel will be closed after this event.
	CompletedEvent EventType = "Completed"
	// AbortedEvent signals that the observer is shutting down because it has been cancelled. All resources might
	// not have been reconciled. The event channel will be closed after this event.
	AbortedEvent EventType = "Aborted"
	// ErrorEvent signals that the observer has encountered an error that it can not recover from. The observer
	// is shutting down and the event channel will be closed after this event.
	ErrorEvent EventType = "Error"
)

// Event defines that type that is passed back through the event channel to notify the caller of changes
// as resources are being observed.
type Event struct {
	// EventType defines the type of event.
	EventType EventType

	// AggregateStatus is the collective status for all the resources. It is computed by the
	// StatusAggregator
	AggregateStatus status.Status

	// Resource is only available for ResourceUpdateEvents. It includes information about the observed resource,
	// including the resource status, any errors and the resource itself (as an unstructured).
	Resource *ObservedResource

	// Error is only available for ErrorEvents. It contains the error that caused the observer to
	// give up.
	Error error
}

// ObservedResource contains information about a resource after we have
// fetched it from the cluster and computed status.
type ObservedResource struct {
	// Identifier contains the information necessary to locate the
	// resource within a cluster.
	Identifier wait.ResourceIdentifier

	// Status is the computed status for this resource.
	Status status.Status

	// Resource contains the actual manifest for the resource that
	// was fetched from the cluster and used to compute status.
	Resource *unstructured.Unstructured

	// Errors contains the error if something went wrong during the
	// process of fetching the resource and computing the status.
	Error error

	// Message is text describing the status of the resource.
	Message string

	// GeneratedResources is a slice of ObservedResource that
	// contains information and status for any generated resources
	// of the current resource.
	GeneratedResources ObservedResources
}

type ObservedResources []*ObservedResource

func (g ObservedResources) Len() int {
	return len(g)
}

func (g ObservedResources) Less(i, j int) bool {
	idI := g[i].Identifier
	idJ := g[j].Identifier

	if idI.Namespace != idJ.Namespace {
		return idI.Namespace < idJ.Namespace
	}
	if idI.GroupKind.Group != idJ.GroupKind.Group {
		return idI.GroupKind.Group < idJ.GroupKind.Group
	}
	if idI.GroupKind.Kind != idJ.GroupKind.Kind {
		return idI.GroupKind.Kind < idJ.GroupKind.Kind
	}
	return idI.Name < idJ.Name
}

func (g ObservedResources) Swap(i, j int) {
	g[i], g[j] = g[j], g[i]
}

// DeepEqual checks if two instances of ObservedResource are identical. This is used
// to determine whether status has changed for a particular resource.
func DeepEqual(or1, or2 *ObservedResource) bool {
	if or1.Identifier != or2.Identifier ||
		or1.Status != or2.Status ||
		or1.Message != or2.Message {
		return false
	}

	if or1.Error != nil && or2.Error != nil && or1.Error.Error() != or2.Error.Error() {
		return false
	}
	if (or1.Error == nil && or2.Error != nil) || (or1.Error != nil && or2.Error == nil) {
		return false
	}

	if len(or1.GeneratedResources) != len(or2.GeneratedResources) {
		return false
	}

	for i := range or1.GeneratedResources {
		if !DeepEqual(or1.GeneratedResources[i], or2.GeneratedResources[i]) {
			return false
		}
	}
	return true
}

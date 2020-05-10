// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package table

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	pe "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/print/table"
)

func newResourceStateCollector(resourceGroups []event.ResourceGroup) *ResourceStateCollector {
	resourceInfos := make(map[object.ObjMetadata]*ResourceInfo)
	for _, group := range resourceGroups {
		action := group.Action
		for _, identifier := range group.Identifiers {
			resourceInfos[identifier] = &ResourceInfo{
				identifier: identifier,
				resourceStatus: &pe.ResourceStatus{
					Identifier: identifier,
					Status:     status.UnknownStatus,
				},
				ResourceAction: action,
			}
		}
	}
	return &ResourceStateCollector{
		resourceInfos: resourceInfos,
	}
}

// ResourceStateCollector consumes the events from the applier
// eventChannel and keeps track of the latest state for all resources.
// It also provides functionality for fetching the latest seen
// state and return it in format that can be used by the
// BaseTablePrinter.
type ResourceStateCollector struct {
	mux sync.RWMutex

	// resourceInfos contains a mapping from the unique
	// resource identifier to a ResourceInfo object that captures
	// the latest state for the given resource.
	resourceInfos map[object.ObjMetadata]*ResourceInfo

	err error
}

// ResourceInfo captures the latest seen state of a single resource.
// This is used for top-level resources that have a ResourceAction
// associated with them.
type ResourceInfo struct {
	// identifier contains the information that identifies a
	// single resource.
	identifier object.ObjMetadata

	// resourceStatus contains the latest status information
	// about the resource.
	resourceStatus *pe.ResourceStatus

	// ResourceAction defines the action we are performing
	// on this particular resource. This can be either Apply
	// or Prune.
	ResourceAction event.ResourceAction

	// ApplyOpResult contains the result after
	// a resource has been applied to the cluster.
	ApplyOpResult *event.ApplyEventOperation

	// Pruned contains information about whether
	// the resources has been pruned.
	Pruned bool
}

// Identifier returns the identifier for the given resource.
func (r *ResourceInfo) Identifier() object.ObjMetadata {
	return r.identifier
}

// ResourceStatus returns the latest seen status for the
// resource.
func (r *ResourceInfo) ResourceStatus() *pe.ResourceStatus {
	return r.resourceStatus
}

// SubResources returns a slice of Resource which contains
// any resources created and managed by this resource.
func (r *ResourceInfo) SubResources() []table.Resource {
	var resources []table.Resource
	for _, res := range r.resourceStatus.GeneratedResources {
		resources = append(resources, &SubResourceInfo{
			resourceStatus: res,
		})
	}
	return resources
}

// SubResourceInfo captures the latest seen state of a
// single subResource, i.e. resources that are created and
// managed by one of the top-level resources we either apply
// or prune.
type SubResourceInfo struct {
	// resourceStatus contains the latest status information
	// about the subResource.
	resourceStatus *pe.ResourceStatus
}

// Identifier returns the identifier for the given subResource.
func (r *SubResourceInfo) Identifier() object.ObjMetadata {
	return r.resourceStatus.Identifier
}

// ResourceStatus returns the latest seen status for the
// subResource.
func (r *SubResourceInfo) ResourceStatus() *pe.ResourceStatus {
	return r.resourceStatus
}

// SubResources returns a slice of Resource which contains
// any resources created and managed by this resource.
func (r *SubResourceInfo) SubResources() []table.Resource {
	var resources []table.Resource
	for _, res := range r.resourceStatus.GeneratedResources {
		resources = append(resources, &SubResourceInfo{
			resourceStatus: res,
		})
	}
	return resources
}

// Listen starts a new goroutine that will listen for events on the
// provided eventChannel and keep track of the latest state for
// the resources. The goroutine will exit when the provided
// eventChannel is closed.
// The function returns a channel. When this channel is closed, the
// goroutine has processed all events in the eventChannel and
// exited.
func (r *ResourceStateCollector) Listen(eventChannel <-chan event.Event) <-chan struct{} {
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		for e := range eventChannel {
			r.processEvent(e)
		}
	}()
	return completed
}

// processEvent processes an event and updates the state.
func (r *ResourceStateCollector) processEvent(e event.Event) {
	r.mux.Lock()
	defer r.mux.Unlock()
	switch e.Type {
	case event.StatusType:
		r.processStatusEvent(e.StatusEvent)
	case event.ApplyType:
		r.processApplyEvent(e.ApplyEvent)
	case event.PruneType:
		r.processPruneEvent(e.PruneEvent)
	case event.ErrorType:
		r.processErrorEvent(e.ErrorEvent.Err)
	}
}

// processStatusEvent handles events pertaining to a status
// update for a resource.
func (r *ResourceStateCollector) processStatusEvent(e pe.Event) {
	if e.EventType == pe.ErrorEvent {
		r.processErrorEvent(e.Error)
		return
	}
	if e.EventType == pe.ResourceUpdateEvent {
		resource := e.Resource
		previous := r.resourceInfos[resource.Identifier]
		previous.resourceStatus = e.Resource
	}
}

// processApplyEvent handles events relating to apply operations
func (r *ResourceStateCollector) processApplyEvent(e event.ApplyEvent) {
	if e.Type == event.ApplyEventResourceUpdate {
		identifier := toIdentifier(e.Object)
		previous, found := r.resourceInfos[identifier]
		if !found {
			return
		}
		previous.ApplyOpResult = &e.Operation
	}
}

// processPruneEvent handles event related to prune operations.
func (r *ResourceStateCollector) processPruneEvent(e event.PruneEvent) {
	if e.Type == event.PruneEventResourceUpdate {
		identifier := toIdentifier(e.Object)
		previous, found := r.resourceInfos[identifier]
		if !found {
			return
		}
		previous.Pruned = true
	}
}

// processErrorEvent handles events for errors.
func (r *ResourceStateCollector) processErrorEvent(err error) {
	if timeoutErr, ok := taskrunner.IsTimeoutError(err); ok {
		r.err = r.handleTimeoutError(timeoutErr)
		return
	}
	r.err = err
}

// handleTimeoutError transforms a TimeoutError into a new error which includes
// information about which resources failed to reach the desired status.
func (r *ResourceStateCollector) handleTimeoutError(timeoutErr taskrunner.TimeoutError) error {
	var errInfo ResourceInfos
	for _, id := range timeoutErr.Identifiers {
		ri, ok := r.resourceInfos[id]
		if !ok {
			continue
		}
		if timeoutErr.Condition.Meets(ri.resourceStatus.Status) {
			continue
		}
		errInfo = append(errInfo, ri)
	}
	sort.Sort(errInfo)
	var b bytes.Buffer
	_, _ = fmt.Fprint(&b, timeoutErr.Error()+"\n")
	for i, ri := range errInfo {
		_, _ = fmt.Fprintf(&b, "%s/%s %s %s", ri.identifier.GroupKind.Kind,
			ri.identifier.Name, ri.resourceStatus.Status, ri.resourceStatus.Message)
		if i != len(errInfo)-1 {
			_, _ = fmt.Fprint(&b, "\n")
		}
	}
	return fmt.Errorf(b.String())
}

// toIdentifier extracts the identifying information from an
// object.
func toIdentifier(o runtime.Object) object.ObjMetadata {
	accessor, _ := meta.Accessor(o)
	return object.ObjMetadata{
		GroupKind: o.GetObjectKind().GroupVersionKind().GroupKind(),
		Namespace: accessor.GetNamespace(),
		Name:      accessor.GetName(),
	}
}

// ResourceState contains the latest state for all the resources.
type ResourceState struct {
	resourceInfos ResourceInfos

	err error
}

// Resources returns a slice containing the latest state
// for each individual resource.
func (r *ResourceState) Resources() []table.Resource {
	var resources []table.Resource
	for _, res := range r.resourceInfos {
		resources = append(resources, res)
	}
	return resources
}

func (r *ResourceState) Error() error {
	return r.err
}

// LatestState returns a ResourceState object that contains
// a copy of the latest state for all resources.
func (r *ResourceStateCollector) LatestState() *ResourceState {
	r.mux.RLock()
	defer r.mux.RUnlock()

	var resourceInfos ResourceInfos
	for _, ri := range r.resourceInfos {
		resourceInfos = append(resourceInfos, &ResourceInfo{
			identifier:     ri.identifier,
			resourceStatus: ri.resourceStatus,
			ResourceAction: ri.ResourceAction,
			ApplyOpResult:  ri.ApplyOpResult,
			Pruned:         ri.Pruned,
		})
	}
	sort.Sort(resourceInfos)

	return &ResourceState{
		resourceInfos: resourceInfos,
		err:           r.err,
	}
}

type ResourceInfos []*ResourceInfo

func (g ResourceInfos) Len() int {
	return len(g)
}

func (g ResourceInfos) Less(i, j int) bool {
	idI := g[i].identifier
	idJ := g[j].identifier

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

func (g ResourceInfos) Swap(i, j int) {
	g[i], g[j] = g[j], g[i]
}

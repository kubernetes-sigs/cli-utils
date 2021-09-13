// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"io/ioutil"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/apply"
	cmddelete "k8s.io/kubectl/pkg/cmd/delete"
	"k8s.io/kubectl/pkg/cmd/util"
	applyerror "sigs.k8s.io/cli-utils/pkg/apply/error"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/dependson"
)

// applyOptions defines the two key functions on the ApplyOptions
// struct that is used by the ApplyTask.
type applyOptions interface {

	// Run applies the resource set with the SetObjects function
	// to the cluster.
	Run() error

	// SetObjects sets the slice of resource (in the form form resourceInfo objects)
	// that will be applied upon invoking the Run function.
	SetObjects([]*resource.Info)
}

// ApplyTask applies the given Objects to the cluster
// by using the ApplyOptions.
type ApplyTask struct {
	TaskName string

	Factory           util.Factory
	InfoHelper        info.InfoHelper
	Mapper            meta.RESTMapper
	Objects           []*unstructured.Unstructured
	Filters           []filter.ValidationFilter
	DryRunStrategy    common.DryRunStrategy
	ServerSideOptions common.ServerSideOptions
}

// applyOptionsFactoryFunc is a factory function for creating a new
// applyOptions implementation. Used to allow unit testing.
var applyOptionsFactoryFunc = newApplyOptions

func (a *ApplyTask) Name() string {
	return a.TaskName
}

func (a *ApplyTask) Action() event.ResourceAction {
	return event.ApplyAction
}

func (a *ApplyTask) Identifiers() []object.ObjMetadata {
	return object.UnstructuredsToObjMetasOrDie(a.Objects)
}

// Start creates a new goroutine that will invoke
// the Run function on the ApplyOptions to update
// the cluster. It will push a TaskResult on the taskChannel
// to signal to the taskrunner that the task has completed (or failed).
// It will also fetch the Generation from each of the applied resources
// after the Run function has completed. This information is then added
// to the taskContext. The generation is increased every time
// the desired state of a resource is changed.
func (a *ApplyTask) Start(taskContext *taskrunner.TaskContext) {
	go func() {
		objects := a.Objects
		klog.V(2).Infof("apply task starting (%d objects)", len(objects))
		// Create a new instance of the applyOptions interface and use it
		// to apply the objects.
		ao, err := applyOptionsFactoryFunc(taskContext.EventChannel(),
			a.ServerSideOptions, a.DryRunStrategy, a.Factory)
		if err != nil {
			if klog.V(4).Enabled() {
				klog.Errorf("error creating ApplyOptions (%s)--returning", err)
			}
			sendBatchApplyEvents(taskContext, objects, err)
			a.sendTaskResult(taskContext)
			return
		}
		for _, obj := range objects {
			// Set the client and mapping fields on the provided
			// info so they can be applied to the cluster.
			info, err := a.InfoHelper.BuildInfo(obj)
			id := object.UnstructuredToObjMetaOrDie(obj)
			if err != nil {
				if klog.V(4).Enabled() {
					klog.Errorf("unable to convert obj to info for %s/%s (%s)--continue",
						obj.GetNamespace(), obj.GetName(), err)
				}
				taskContext.EventChannel() <- createApplyFailedEvent(id,
					applyerror.NewUnknownTypeError(err))
				taskContext.CaptureResourceFailure(id)
				continue
			}
			// Check filters to see if we're prevented from applying.
			var filtered bool
			var filterErr error
			for _, filter := range a.Filters {
				klog.V(6).Infof("apply filter %s: %s", filter.Name(), id)
				var reason string
				filtered, reason, filterErr = filter.Filter(obj)
				if filterErr != nil {
					if klog.V(5).Enabled() {
						klog.Errorf("error during %s, (%s): %s", filter.Name(), id, filterErr)
					}
					taskContext.EventChannel() <- createApplyFailedEvent(id, filterErr)
					taskContext.CaptureResourceFailure(id)
					break
				}
				if filtered {
					klog.V(4).Infof("apply filtered by %s because (%s): %s", filter.Name(), reason, id)
					taskContext.EventChannel() <- createApplyEvent(id, event.Unchanged, obj)
					taskContext.CaptureResourceFailure(id)
					break
				}
			}
			if filtered || filterErr != nil {
				continue
			}
			// Before applying, fill in apply-time mutations for this object.
			if err := mutate(obj, taskContext); err != nil {
				klog.V(5).Infof("apply mutate error--skip: %s/%s", id.Namespace, id.Name)
				continue
			}
			ao.SetObjects([]*resource.Info{info})
			klog.V(5).Infof("applying %s/%s...", info.Namespace, info.Name)
			err = ao.Run()
			if err != nil && a.ServerSideOptions.ServerSideApply && isAPIService(obj) && isStreamError(err) {
				// Server-side Apply doesn't work with APIService before k8s 1.21
				// https://github.com/kubernetes/kubernetes/issues/89264
				// Thus APIService is handled specially using client-side apply.
				err = clientSideApply(info, taskContext.EventChannel(), a.DryRunStrategy, a.Factory)
			}
			if err != nil {
				if klog.V(4).Enabled() {
					klog.Errorf("error applying (%s/%s) %s", info.Namespace, info.Name, err)
				}
				taskContext.EventChannel() <- createApplyFailedEvent(id,
					applyerror.NewApplyRunError(err))
				taskContext.CaptureResourceFailure(id)
			} else if info.Object != nil {
				klog.V(5).Infof("successfully applied: %s/%s", info.Namespace, info.Name)
				taskContext.ResourceApplied(id, object.InfoToUnstructured(info))
			}
		}
		a.sendTaskResult(taskContext)
	}()
}

// Mutate updates the passed object with values found in applied objects
// stored in the passed taskContext. Returns an error if one occurs
// during the mutation.
func mutate(obj *unstructured.Unstructured, taskContext *taskrunner.TaskContext) error {
	if found := dependson.HasAnnotation(obj); !found {
		return nil
	}
	dependsOnIds, err := dependson.ReadAnnotation(obj)
	if err != nil {
		return err
	}
	for _, depID := range dependsOnIds {
		depObj := taskContext.GetAppliedResource(depID)
		if depObj == nil {
			return fmt.Errorf("expected obj to already be applied: %s/%s", depID.Namespace, depID.Name)
		}
		// Retrieve depends-on object here to get the updated values. Check
		// the "Ready" state after retrieval. If it is not "Ready", then the
		// WaitTask timed out and this object failed--return an error. There
		// is no polling required.
		klog.Infof("mutate %s/%s field here: source depends-on object: %s/%s",
			obj.GetNamespace(), obj.GetName(), depObj.GetNamespace(), depObj.GetName())
	}
	return nil
}

func newApplyOptions(eventChannel chan event.Event, serverSideOptions common.ServerSideOptions,
	strategy common.DryRunStrategy, factory util.Factory) (applyOptions, error) {
	discovery, err := factory.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	dynamic, err := factory.DynamicClient()
	if err != nil {
		return nil, err
	}
	emptyString := ""
	return &apply.ApplyOptions{
		VisitedNamespaces: sets.NewString(),
		VisitedUids:       sets.NewString(),
		Overwrite:         true, // Normally set in apply.NewApplyOptions
		OpenAPIPatch:      true, // Normally set in apply.NewApplyOptions
		Recorder:          genericclioptions.NoopRecorder{},
		IOStreams: genericclioptions.IOStreams{
			Out:    ioutil.Discard,
			ErrOut: ioutil.Discard, // TODO: Warning for no lastConfigurationAnnotation
			// is printed directly to stderr in ApplyOptions. We
			// should turn that into a warning on the event channel.
		},
		// FilenameOptions are not needed since we don't use the ApplyOptions
		// to read manifests.
		DeleteOptions: &cmddelete.DeleteOptions{},
		PrintFlags: &genericclioptions.PrintFlags{
			OutputFormat: &emptyString,
		},
		// Server-side apply if flag set or server-side dry run.
		ServerSideApply: strategy.ServerDryRun() || serverSideOptions.ServerSideApply,
		ForceConflicts:  serverSideOptions.ForceConflicts,
		FieldManager:    serverSideOptions.FieldManager,
		DryRunStrategy:  strategy.Strategy(),
		ToPrinter: (&KubectlPrinterAdapter{
			ch: eventChannel,
		}).toPrinterFunc(),
		DynamicClient:  dynamic,
		DryRunVerifier: resource.NewDryRunVerifier(dynamic, discovery),
	}, nil
}

func (a *ApplyTask) sendTaskResult(taskContext *taskrunner.TaskContext) {
	taskContext.TaskChannel() <- taskrunner.TaskResult{}
}

// ClearTimeout is not supported by the ApplyTask.
func (a *ApplyTask) ClearTimeout() {}

// createApplyEvent is a helper function to package an apply event for a single resource.
func createApplyEvent(id object.ObjMetadata, operation event.ApplyEventOperation, resource *unstructured.Unstructured) event.Event {
	return event.Event{
		Type: event.ApplyType,
		ApplyEvent: event.ApplyEvent{
			Identifier: id,
			Operation:  operation,
			Resource:   resource,
		},
	}
}

func createApplyFailedEvent(id object.ObjMetadata, err error) event.Event {
	return event.Event{
		Type: event.ApplyType,
		ApplyEvent: event.ApplyEvent{
			Identifier: id,
			Error:      err,
		},
	}
}

// sendBatchApplyEvents is a helper function to send out multiple apply events for
// a list of resources when failed to initialize the apply process.
func sendBatchApplyEvents(taskContext *taskrunner.TaskContext, objects []*unstructured.Unstructured, err error) {
	for _, obj := range objects {
		id := object.UnstructuredToObjMetaOrDie(obj)
		taskContext.EventChannel() <- createApplyFailedEvent(id,
			applyerror.NewInitializeApplyOptionError(err))
		taskContext.CaptureResourceFailure(id)
	}
}

func isAPIService(obj *unstructured.Unstructured) bool {
	gk := obj.GroupVersionKind().GroupKind()
	return gk.Group == "apiregistration.k8s.io" && gk.Kind == "APIService"
}

// isStreamError checks if the error is a StreamError. Since kubectl wraps the actual StreamError,
// we can't check the error type.
func isStreamError(err error) bool {
	return strings.Contains(err.Error(), "stream error: stream ID ")
}

func clientSideApply(info *resource.Info, eventChannel chan event.Event, strategy common.DryRunStrategy, factory util.Factory) error {
	ao, err := applyOptionsFactoryFunc(eventChannel, common.ServerSideOptions{ServerSideApply: false}, strategy, factory)
	if err != nil {
		return err
	}
	ao.SetObjects([]*resource.Info{info})
	return ao.Run()
}

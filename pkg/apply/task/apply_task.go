// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/cmd/apply"
	cmddelete "k8s.io/kubectl/pkg/cmd/delete"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util"
	applyerror "sigs.k8s.io/cli-utils/pkg/apply/error"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/mutator"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
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

	DynamicClient     dynamic.Interface
	OpenAPIGetter     discovery.OpenAPISchemaInterface
	InfoHelper        info.Helper
	Mapper            meta.RESTMapper
	Objects           object.UnstructuredSet
	Filters           []filter.ValidationFilter
	Mutators          []mutator.Interface
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

func (a *ApplyTask) Identifiers() object.ObjMetadataSet {
	return object.UnstructuredSetToObjMetadataSet(a.Objects)
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
		// TODO: pipe Context through TaskContext
		ctx := context.TODO()
		objects := a.Objects
		klog.V(2).Infof("apply task starting (name: %q, objects: %d)",
			a.Name(), len(objects))
		for _, obj := range objects {
			// Set the client and mapping fields on the provided
			// info so they can be applied to the cluster.
			info, err := a.InfoHelper.BuildInfo(obj)
			// BuildInfo strips path annotations.
			// Use modified object for filters, mutations, and events.
			obj = info.Object.(*unstructured.Unstructured)
			id := object.UnstructuredToObjMetadata(obj)
			if err != nil {
				err = applyerror.NewUnknownTypeError(err)
				if klog.V(4).Enabled() {
					// only log event emitted errors if the verbosity > 4
					klog.Errorf("apply task errored (object: %s): unable to convert obj to info: %v", id, err)
				}
				taskContext.SendEvent(a.createApplyFailedEvent(id, err))
				taskContext.InventoryManager().AddFailedApply(id)
				continue
			}

			// Check filters to see if we're prevented from applying.
			var filterErr error
			for _, applyFilter := range a.Filters {
				klog.V(6).Infof("apply filter evaluating (filter: %s, object: %s)", applyFilter.Name(), id)
				filterErr = applyFilter.Filter(obj)
				if filterErr != nil {
					var fatalErr *filter.FatalError
					if errors.As(filterErr, &fatalErr) {
						if klog.V(4).Enabled() {
							// only log event emitted errors if the verbosity > 4
							klog.Errorf("apply filter errored (filter: %s, object: %s): %v", applyFilter.Name(), id, fatalErr.Err)
						}
						taskContext.SendEvent(a.createApplyFailedEvent(id, fatalErr))
						taskContext.InventoryManager().AddFailedApply(id)
						break
					}
					klog.V(4).Infof("apply filtered (filter: %s, object: %s): %v", applyFilter.Name(), id, filterErr)
					taskContext.SendEvent(a.createApplySkippedEvent(id, obj, filterErr))
					taskContext.InventoryManager().AddSkippedApply(id)
					break
				}
			}
			if filterErr != nil {
				continue
			}

			// Execute mutators, if any apply
			err = a.mutate(ctx, obj)
			if err != nil {
				if klog.V(4).Enabled() {
					// only log event emitted errors if the verbosity > 4
					klog.Errorf("apply mutation errored (object: %s): %v", id, err)
				}
				taskContext.SendEvent(a.createApplyFailedEvent(id, err))
				taskContext.InventoryManager().AddFailedApply(id)
				continue
			}

			// Create a new instance of the applyOptions interface and use it
			// to apply the objects.
			ao := applyOptionsFactoryFunc(a.Name(), taskContext.EventChannel(),
				a.ServerSideOptions, a.DryRunStrategy, a.DynamicClient, a.OpenAPIGetter)
			ao.SetObjects([]*resource.Info{info})
			klog.V(5).Infof("applying object: %v", id)
			if mutationIgnored(*obj) {
				err = applyMutationIgnoredObject(ao, info, a.ServerSideOptions.FieldManager)
			} else {
				err = ao.Run()
				if err != nil && a.ServerSideOptions.ServerSideApply && isAPIService(obj) && isStreamError(err) {
					// Server-side Apply doesn't work with APIService before k8s 1.21
					// https://github.com/kubernetes/kubernetes/issues/89264
					// Thus APIService is handled specially using client-side apply.
					err = a.clientSideApply(info, taskContext.EventChannel())
				}
			}
			if err != nil {
				err = applyerror.NewApplyRunError(err)
				if klog.V(4).Enabled() {
					// only log event emitted errors if the verbosity > 4
					klog.Errorf("apply errored (object: %s): %v", id, err)
				}
				taskContext.SendEvent(a.createApplyFailedEvent(id, err))
				taskContext.InventoryManager().AddFailedApply(id)
			} else if info.Object != nil {
				acc, err := meta.Accessor(info.Object)
				if err == nil {
					uid := acc.GetUID()
					gen := acc.GetGeneration()
					taskContext.InventoryManager().AddSuccessfulApply(id, uid, gen)
				}
			}
		}
		a.sendTaskResult(taskContext)
	}()
}

func mutationIgnored(obj unstructured.Unstructured) bool {
	if obj.GetAnnotations() == nil {
		return false
	}
	value, ok := obj.GetAnnotations()[common.LifecycleMutationAnnotation]
	return ok && value == common.IgnoreMutation
}

func applyMutationIgnoredObject(applyOpts applyOptions, info *resource.Info, fieldManager string) error {
	helper := resource.NewHelper(info.Client, info.Mapping).
		DryRun(false).
		WithFieldManager(fieldManager)

	ao, ok := applyOpts.(*apply.ApplyOptions)
	if !ok {
		return fmt.Errorf("expecting ApplyOptions type, but got %T", applyOpts)
	}

	if err := info.Get(); err != nil {
		if !apierrors.IsNotFound(err) {
			return cmdutil.AddSourceToErr(fmt.Sprintf("retrieving current configuration of:\n%s\nfrom server for:", info.String()), info.Source, err)
		}

		// Create the resource if it doesn't exist
		return createIgnoredObjectIfAbsent(ao, info, helper)
	}

	// Patch the resource based on the patchData if it already exists.
	return patchIgnoredObject(ao, info, helper)
}

func createIgnoredObjectIfAbsent(ao *apply.ApplyOptions, info *resource.Info, helper *resource.Helper) error {
	// First, remove the patch-ignore annotation
	if err := removeMutationPatchAnnotation(info.Object); err != nil {
		return cmdutil.AddSourceToErr("creating", info.Source, err)
	}

	// Second, update the annotation used by kubectl apply
	if err := util.CreateApplyAnnotation(info.Object, unstructured.UnstructuredJSONScheme); err != nil {
		return cmdutil.AddSourceToErr("creating", info.Source, err)
	}
	// Then create the resource and skip the three-way merge
	obj, err := helper.Create(info.Namespace, true, info.Object)
	if err != nil {
		return cmdutil.AddSourceToErr("creating", info.Source, err)
	}
	info.Refresh(obj, true)

	if err := ao.MarkObjectVisited(info); err != nil {
		return err
	}

	return printObject(ao, info, "created")
}

func patchIgnoredObject(ao *apply.ApplyOptions, info *resource.Info, helper *resource.Helper) error {
	if err := ao.MarkObjectVisited(info); err != nil {
		return err
	}

	metadata, _ := meta.Accessor(info.Object)
	annotationMap := metadata.GetAnnotations()

	if annotationMap == nil {
		return printObject(ao, info, "unchanged")
	}
	patchData, ok := annotationMap[common.IgnoreMutationPatchAnnotation]
	if !ok || patchData == "" || patchData == "{}" {
		return printObject(ao, info, "unchanged")
	}

	patchedObject, err := helper.Patch(
		info.Namespace,
		info.Name,
		types.MergePatchType,
		[]byte(patchData),
		nil,
	)

	if err != nil {
		return cmdutil.AddSourceToErr(fmt.Sprintf("applying patch:\n%s\nto:\n%v\nfor:", patchData, info), info.Source, err)
	}

	info.Refresh(patchedObject, true)

	if metadata != nil && metadata.GetDeletionTimestamp() != nil {
		// just warn the user about the conflict
		klog.Warningf("Warning: Detected changes to resource %s/%s which is currently being deleted.", metadata.GetNamespace(), metadata.GetName())
	}

	return printObject(ao, info, "configured")
}

func removeMutationPatchAnnotation(obj runtime.Object) error {
	metadataAccessor := meta.NewAccessor()
	annotations, err := metadataAccessor.Annotations(obj)
	if err != nil {
		return err
	}
	if annotations == nil {
		return nil
	}
	delete(annotations, common.IgnoreMutationPatchAnnotation)
	return metadataAccessor.SetAnnotations(obj, annotations)
}

func printObject(ao *apply.ApplyOptions, info *resource.Info, message string) error {
	printer, err := ao.ToPrinter(message)
	if err != nil {
		return err
	}
	if err = printer.PrintObj(info.Object, ao.Out); err != nil {
		return err
	}
	return nil
}

func newApplyOptions(taskName string, eventChannel chan<- event.Event, serverSideOptions common.ServerSideOptions,
	strategy common.DryRunStrategy, dynamicClient dynamic.Interface,
	openAPIGetter discovery.OpenAPISchemaInterface) applyOptions {
	emptyString := ""
	return &apply.ApplyOptions{
		VisitedNamespaces: sets.New[string](),
		VisitedUids:       sets.New[types.UID](),
		Overwrite:         true, // Normally set in apply.NewApplyOptions
		OpenAPIPatch:      true, // Normally set in apply.NewApplyOptions
		Recorder:          genericclioptions.NoopRecorder{},
		IOStreams: genericiooptions.IOStreams{
			Out:    io.Discard,
			ErrOut: io.Discard, // TODO: Warning for no lastConfigurationAnnotation
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
			ch:        eventChannel,
			groupName: taskName,
		}).toPrinterFunc(),
		DynamicClient: dynamicClient,
	}
}

func (a *ApplyTask) sendTaskResult(taskContext *taskrunner.TaskContext) {
	klog.V(2).Infof("apply task completing (name: %q)", a.Name())
	taskContext.TaskChannel() <- taskrunner.TaskResult{}
}

// Cancel is not supported by the ApplyTask.
func (a *ApplyTask) Cancel(_ *taskrunner.TaskContext) {}

// StatusUpdate is not supported by the ApplyTask.
func (a *ApplyTask) StatusUpdate(_ *taskrunner.TaskContext, _ object.ObjMetadata) {}

// mutate loops through the mutator list and executes them on the object.
func (a *ApplyTask) mutate(ctx context.Context, obj *unstructured.Unstructured) error {
	id := object.UnstructuredToObjMetadata(obj)
	for _, mutator := range a.Mutators {
		klog.V(6).Infof("apply mutator %s: %s", mutator.Name(), id)
		mutated, reason, err := mutator.Mutate(ctx, obj)
		if err != nil {
			return fmt.Errorf("failed to mutate %q with %q: %w", id, mutator.Name(), err)
		}
		if mutated {
			klog.V(4).Infof("resource mutated (mutator: %q, resource: %q, reason: %q)", mutator.Name(), id, reason)
		}
	}
	return nil
}

func (a *ApplyTask) createApplyFailedEvent(id object.ObjMetadata, err error) event.Event {
	return event.Event{
		Type: event.ApplyType,
		ApplyEvent: event.ApplyEvent{
			GroupName:  a.Name(),
			Identifier: id,
			Status:     event.ApplyFailed,
			Error:      err,
		},
	}
}

func (a *ApplyTask) createApplySkippedEvent(id object.ObjMetadata, resource *unstructured.Unstructured, err error) event.Event {
	return event.Event{
		Type: event.ApplyType,
		ApplyEvent: event.ApplyEvent{
			GroupName:  a.Name(),
			Identifier: id,
			Status:     event.ApplySkipped,
			Resource:   resource,
			Error:      err,
		},
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

func (a *ApplyTask) clientSideApply(info *resource.Info, eventChannel chan<- event.Event) error {
	ao := applyOptionsFactoryFunc(a.Name(), eventChannel, common.ServerSideOptions{ServerSideApply: false}, a.DryRunStrategy, a.DynamicClient, a.OpenAPIGetter)
	ao.SetObjects([]*resource.Info{info})
	return ao.Run()
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"io/ioutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/cmd/delete"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/slice"
	applyerror "sigs.k8s.io/cli-utils/pkg/apply/error"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
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
	Factory           util.Factory
	InfoHelper        info.InfoHelper
	Mapper            meta.RESTMapper
	Objects           []*unstructured.Unstructured
	CRDs              []*unstructured.Unstructured
	DryRunStrategy    common.DryRunStrategy
	ServerSideOptions common.ServerSideOptions
	InventoryPolicy   inventory.InventoryPolicy
	InvInfo           inventory.InventoryInfo
}

// applyOptionsFactoryFunc is a factory function for creating a new
// applyOptions implementation. Used to allow unit testing.
var applyOptionsFactoryFunc = newApplyOptions

// getClusterObj gets the cluster object. Used for allow unit testing.
var getClusterObj = getClusterObject

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

		// If this is a dry run, we need to handle situations where
		// we have a CRD and a CR in the same resource set, but the CRD
		// will not actually have been applied when we reach the CR.
		if a.DryRunStrategy.ClientOrServerDryRun() {
			// Find all resources in the set that doesn't exist in the
			// RESTMapper, but where we do have the CRD for the type in
			// the resource set.
			objs, objsWithCRD, err := a.filterCRsWithCRDInSet(objects)
			if err != nil {
				sendBatchApplyEvents(taskContext, objs, err)
				a.sendTaskResult(taskContext)
				return
			}

			// Just send the apply event here. We know it must be a
			// Created event since the type didn't already exist in the
			// cluster.
			for _, obj := range objsWithCRD {
				taskContext.EventChannel() <- createApplyEvent(object.UnstructuredToObjMeta(obj), event.Created, nil)
			}
			// Update the resource set to no longer include the CRs.
			objects = objs
		}

		// ApplyOptions doesn't allow an empty set of resources, so check
		// for that here. It could happen if this is dry-run and we removed
		// all resources in the previous step.
		if len(objects) == 0 {
			a.sendTaskResult(taskContext)
			return
		}

		// Create a new instance of the applyOptions interface and use it
		// to apply the objects.
		ao, dynamic, err := applyOptionsFactoryFunc(taskContext.EventChannel(),
			a.ServerSideOptions, a.DryRunStrategy, a.Factory)
		if err != nil {
			sendBatchApplyEvents(taskContext, objects, err)
			a.sendTaskResult(taskContext)
			return
		}

		var infos []*resource.Info
		for _, obj := range objects {
			// Set the client and mapping fields on the provided
			// info so they can be applied to the cluster.
			info, err := a.InfoHelper.BuildInfo(obj)
			if err != nil {
				taskContext.EventChannel() <- createApplyEvent(
					object.UnstructuredToObjMeta(obj), event.Failed, applyerror.NewUnknownTypeError(err))
				continue
			}

			clusterObj, err := getClusterObj(dynamic, info)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					taskContext.EventChannel() <- createApplyEvent(
						object.UnstructuredToObjMeta(obj),
						event.Unchanged,
						err)
					continue
				}
			}
			infos = append(infos, info)
			canApply, err := inventory.CanApply(a.InvInfo, clusterObj, a.InventoryPolicy)
			if !canApply {
				taskContext.EventChannel() <- createApplyEvent(
					object.UnstructuredToObjMeta(obj),
					event.Unchanged,
					err)
				continue
			}
			// add the inventory annotation to the resource being applied.
			inventory.AddInventoryIDAnnotation(obj, a.InvInfo)
			ao.SetObjects([]*resource.Info{info})
			err = ao.Run()
			if err != nil {
				taskContext.EventChannel() <- createApplyEvent(
					object.UnstructuredToObjMeta(obj), event.Failed, applyerror.NewApplyRunError(err))
			}
		}

		// Fetch the Generation from all Infos after they have been
		// applied.
		for _, inf := range infos {
			id, err := object.InfoToObjMeta(inf)
			if err != nil {
				continue
			}
			if inf.Object != nil {
				acc, err := meta.Accessor(inf.Object)
				if err != nil {
					continue
				}
				uid := acc.GetUID()
				gen := acc.GetGeneration()
				taskContext.ResourceApplied(id, uid, gen)
			}
		}
		a.sendTaskResult(taskContext)
	}()
}

func newApplyOptions(eventChannel chan event.Event, serverSideOptions common.ServerSideOptions,
	strategy common.DryRunStrategy, factory util.Factory) (applyOptions, dynamic.Interface, error) {
	discovery, err := factory.ToDiscoveryClient()
	if err != nil {
		return nil, nil, err
	}
	dynamic, err := factory.DynamicClient()
	if err != nil {
		return nil, nil, err
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
		DeleteOptions: &delete.DeleteOptions{},
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
	}, dynamic, nil
}

func getClusterObject(p dynamic.Interface, info *resource.Info) (*unstructured.Unstructured, error) {
	namespacedClient := p.Resource(info.Mapping.Resource).Namespace(info.Namespace)
	return namespacedClient.Get(context.TODO(), info.Name, metav1.GetOptions{})
}

func (a *ApplyTask) sendTaskResult(taskContext *taskrunner.TaskContext) {
	taskContext.TaskChannel() <- taskrunner.TaskResult{}
}

// filterCRsWithCRDInSet loops through all the resources and filters out the
// resources that doesn't exist in the RESTMapper, but where we do have a CRD
// in the resource set that defines the needed type. It returns two slices,
// the seconds contains the resources that meets the above criteria while the
// first slice contains the remaining resources.
func (a *ApplyTask) filterCRsWithCRDInSet(objects []*unstructured.Unstructured) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	var objs []*unstructured.Unstructured
	var objsWithCRD []*unstructured.Unstructured

	crdsInfo := buildCRDsInfo(a.CRDs)
	for _, obj := range objects {
		gvk := obj.GroupVersionKind()

		// First check if we find the type in the RESTMapper.
		//TODO: Maybe we do care if there is a new version of the CRD?
		_, err := a.Mapper.RESTMapping(gvk.GroupKind())
		if err != nil && !meta.IsNoMatchError(err) {
			return objs, objsWithCRD, err
		}

		// If we can't find the type in the RESTMapper, but we do have the
		// CRD in the set of resources, filter out the object.
		if meta.IsNoMatchError(err) && crdsInfo.includesCRDForCR(obj) {
			objsWithCRD = append(objsWithCRD, obj)
			continue
		}

		// If the resource is in the RESTMapper, or it is not there but we
		// also don't have the CRD, just keep the resource.
		objs = append(objs, obj)
	}
	return objs, objsWithCRD, nil
}

type crdsInfo struct {
	crds []crdInfo
}

// includesCRDForCR checks if we have information about a CRD that defines
// the types needed for the provided CR.
func (c *crdsInfo) includesCRDForCR(cr *unstructured.Unstructured) bool {
	gvk := cr.GroupVersionKind()
	for _, crd := range c.crds {
		if gvk.Group == crd.group &&
			gvk.Kind == crd.kind &&
			slice.ContainsString(crd.versions, gvk.Version, nil) {
			return true
		}
	}
	return false
}

type crdInfo struct {
	group    string
	kind     string
	versions []string
}

func buildCRDsInfo(crds []*unstructured.Unstructured) *crdsInfo {
	var crdsInf []crdInfo
	for _, crd := range crds {
		group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
		kind, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "kind")

		var versions []string
		crdVersions, _, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
		for _, ver := range crdVersions {
			verObj := ver.(map[string]interface{})
			version, _, _ := unstructured.NestedString(verObj, "name")
			versions = append(versions, version)
		}
		crdsInf = append(crdsInf, crdInfo{
			kind:     kind,
			group:    group,
			versions: versions,
		})
	}
	return &crdsInfo{
		crds: crdsInf,
	}
}

// ClearTimeout is not supported by the ApplyTask.
func (a *ApplyTask) ClearTimeout() {}

// createApplyEvent is a helper function to package an apply event for a single resource.
func createApplyEvent(id object.ObjMetadata, operation event.ApplyEventOperation, err error) event.Event {
	return event.Event{
		Type: event.ApplyType,
		ApplyEvent: event.ApplyEvent{
			Type:       event.ApplyEventResourceUpdate,
			Operation:  operation,
			Identifier: id,
			Error:      err,
		},
	}
}

// sendBatchApplyEvents is a helper function to send out multiple apply events for
// a list of resources when failed to initialize the apply process.
func sendBatchApplyEvents(taskContext *taskrunner.TaskContext, objects []*unstructured.Unstructured, err error) {
	for _, obj := range objects {
		taskContext.EventChannel() <- createApplyEvent(
			object.UnstructuredToObjMeta(obj), event.Failed, applyerror.NewInitializeApplyOptionError(err))
	}
}

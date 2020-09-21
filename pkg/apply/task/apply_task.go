// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"k8s.io/kubectl/pkg/util/slice"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// ApplyTask applies the given Objects to the cluster
// by using the ApplyOptions.
type ApplyTask struct {
	ApplyOptions   applyOptions
	InfoHelper     info.InfoHelper
	Mapper         meta.RESTMapper
	Objects        []*resource.Info
	CRDs           []*resource.Info
	DryRunStrategy common.DryRunStrategy
}

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
		// Update the dry-run field on the Applier.
		a.setApplyOptionsFields(taskContext.EventChannel())

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
				a.sendTaskResult(taskContext, err)
				return
			}

			// Just send the apply event here. We know it must be a
			// Created event since the type didn't already exist in the
			// cluster.
			for _, obj := range objsWithCRD {
				taskContext.EventChannel() <- event.Event{
					Type: event.ApplyType,
					ApplyEvent: event.ApplyEvent{
						Type:      event.ApplyEventResourceUpdate,
						Operation: event.Created,
						Object:    obj.Object,
					},
				}
			}
			// Update the resource set to no longer include the CRs.
			objects = objs
		}

		// ApplyOptions doesn't allow an empty set of resources, so check
		// for that here. It could happen if this is dry-run and we removed
		// all resources in the previous step.
		if len(objects) == 0 {
			a.sendTaskResult(taskContext, nil)
			return
		}

		// Set the client and mapping fields on the provided
		// infos so they can be applied to the cluster.
		err := a.InfoHelper.UpdateInfos(objects)
		if err != nil {
			a.sendTaskResult(taskContext, err)
			return
		}
		a.ApplyOptions.SetObjects(objects)
		err = a.ApplyOptions.Run()
		if err != nil {
			a.sendTaskResult(taskContext, err)
			return
		}
		// Fetch the Generation from all Infos after they have been
		// applied.
		//TODO: This isn't really needed if we are doing dry-run.
		for _, obj := range objects {
			id, err := object.InfoToObjMeta(obj)
			if err != nil {
				continue
			}
			if obj.Object != nil {
				acc, err := meta.Accessor(obj.Object)
				if err != nil {
					continue
				}
				uid := acc.GetUID()
				gen := acc.GetGeneration()
				taskContext.ResourceApplied(id, uid, gen)
			}
		}
		a.sendTaskResult(taskContext, nil)
	}()
}

func (a *ApplyTask) sendTaskResult(taskContext *taskrunner.TaskContext, err error) {
	taskContext.TaskChannel() <- taskrunner.TaskResult{
		Err: err,
	}
}

func (a *ApplyTask) setApplyOptionsFields(eventChannel chan event.Event) {
	if ao, ok := a.ApplyOptions.(*apply.ApplyOptions); ok {
		ao.DryRun = a.DryRunStrategy.ClientDryRun()
		ao.ServerDryRun = a.DryRunStrategy.ServerDryRun()
		adapter := &KubectlPrinterAdapter{
			ch: eventChannel,
		}
		// The adapter is used to intercept what is meant to be printing
		// in the ApplyOptions, and instead turn those into events.
		ao.ToPrinter = adapter.toPrinterFunc()
	}
}

// filterCRsWithCRDInSet loops through all the resources and filters out the
// resources that doesn't exist in the RESTMapper, but where we do have a CRD
// in the resource set that defines the needed type. It returns two slices,
// the seconds contains the resources that meets the above criteria while the
// first slice contains the remaining resources.
func (a *ApplyTask) filterCRsWithCRDInSet(objects []*resource.Info) ([]*resource.Info, []*resource.Info, error) {
	var objs []*resource.Info
	var objsWithCRD []*resource.Info

	crdsInfo := buildCRDsInfo(a.CRDs)
	for _, obj := range objects {
		gvk := obj.Object.GetObjectKind().GroupVersionKind()

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
func (c *crdsInfo) includesCRDForCR(cr *resource.Info) bool {
	gvk := cr.Object.GetObjectKind().GroupVersionKind()
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

func buildCRDsInfo(crds []*resource.Info) *crdsInfo {
	var crdsInf []crdInfo
	for _, crd := range crds {
		u := crd.Object.(*unstructured.Unstructured)
		group, _, _ := unstructured.NestedString(u.Object, "spec", "group")
		kind, _, _ := unstructured.NestedString(u.Object, "spec", "names", "kind")

		var versions []string
		crdVersions, _, _ := unstructured.NestedSlice(u.Object, "spec", "versions")
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

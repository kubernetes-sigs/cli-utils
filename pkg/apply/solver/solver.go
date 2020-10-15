// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// The solver package is responsible for constructing a
// taskqueue based on the set of resources that should be
// applied.
// This involves setting up the appropriate sequence of
// apply, wait and prune tasks so any dependencies between
// resources doesn't cause a later apply operation to
// fail.
// Currently this package assumes that the resources have
// already been sorted in the appropriate order. We might
// want to consider moving the sorting functionality into
// this package.
package solver

import (
	"time"

	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/info"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/task"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	pollevent "sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type TaskQueueSolver struct {
	PruneOptions *prune.PruneOptions
	InfoHelper   info.InfoHelper
	Factory      util.Factory
	Mapper       meta.RESTMapper
}

type Options struct {
	ReconcileTimeout       time.Duration
	Prune                  bool
	DryRunStrategy         common.DryRunStrategy
	PrunePropagationPolicy metav1.DeletionPropagation
	PruneTimeout           time.Duration
}

type resourceObjects interface {
	InfosForApply() []*resource.Info
	InfosForPrune() []*resource.Info
	IdsForApply() []object.ObjMetadata
	IdsForPrune() []object.ObjMetadata
}

// BuildTaskQueue takes a set of resources in the form of info objects
// and constructs the task queue. The options parameter allows
// customization of how the task queue are built.
func (t *TaskQueueSolver) BuildTaskQueue(ro resourceObjects,
	o Options) chan taskrunner.Task {
	var tasks []taskrunner.Task
	remainingInfos := ro.InfosForApply()

	crdSplitRes, hasCRDs := splitAfterCRDs(remainingInfos)
	if hasCRDs {
		tasks = append(tasks, &task.ApplyTask{
			Objects:        append(crdSplitRes.before, crdSplitRes.crds...),
			CRDs:           crdSplitRes.crds,
			DryRunStrategy: o.DryRunStrategy,
			InfoHelper:     t.InfoHelper,
			Factory:        t.Factory,
			Mapper:         t.Mapper,
		})
		if !o.DryRunStrategy.ClientOrServerDryRun() {
			objs, _ := object.InfosToObjMetas(crdSplitRes.crds)
			tasks = append(tasks, taskrunner.NewWaitTask(
				objs,
				taskrunner.AllCurrent,
				1*time.Minute),
				&task.ResetRESTMapperTask{
					Mapper: t.Mapper,
				})
		}
		remainingInfos = crdSplitRes.after
	}

	tasks = append(tasks,
		&task.ApplyTask{
			Objects:        remainingInfos,
			CRDs:           crdSplitRes.crds,
			DryRunStrategy: o.DryRunStrategy,
			InfoHelper:     t.InfoHelper,
			Factory:        t.Factory,
			Mapper:         t.Mapper,
		},
		&task.SendEventTask{
			Event: event.Event{
				Type: event.ApplyType,
				ApplyEvent: event.ApplyEvent{
					Type: event.ApplyEventCompleted,
				},
			},
		},
	)

	if !o.DryRunStrategy.ClientOrServerDryRun() && o.ReconcileTimeout != time.Duration(0) {
		tasks = append(tasks,
			taskrunner.NewWaitTask(
				ro.IdsForApply(),
				taskrunner.AllCurrent,
				o.ReconcileTimeout),
			&task.SendEventTask{
				Event: event.Event{
					Type: event.StatusType,
					StatusEvent: pollevent.Event{
						EventType: pollevent.CompletedEvent,
					},
				},
			},
		)
	}

	if o.Prune {
		tasks = append(tasks,
			&task.PruneTask{
				Objects:           ro.InfosForPrune(),
				PruneOptions:      t.PruneOptions,
				PropagationPolicy: o.PrunePropagationPolicy,
				DryRunStrategy:    o.DryRunStrategy,
			},
			&task.SendEventTask{
				Event: event.Event{
					Type: event.PruneType,
					PruneEvent: event.PruneEvent{
						Type: event.PruneEventCompleted,
					},
				},
			},
		)

		if !o.DryRunStrategy.ClientOrServerDryRun() && o.PruneTimeout != time.Duration(0) {
			tasks = append(tasks,
				taskrunner.NewWaitTask(
					ro.IdsForPrune(),
					taskrunner.AllNotFound,
					o.PruneTimeout),
				&task.SendEventTask{
					Event: event.Event{
						Type: event.StatusType,
						StatusEvent: pollevent.Event{
							EventType: pollevent.CompletedEvent,
						},
					},
				},
			)
		}
	}

	return tasksToQueue(tasks)
}

func tasksToQueue(tasks []taskrunner.Task) chan taskrunner.Task {
	taskQueue := make(chan taskrunner.Task, len(tasks))
	for _, t := range tasks {
		taskQueue <- t
	}
	return taskQueue
}

type crdSplitResult struct {
	before []*resource.Info
	after  []*resource.Info
	crds   []*resource.Info
}

// splitAfterCRDs takes a sorted slice of infos and splits it into
// three parts; resources before CRDs, the CRDs themselves, and finally
// all the resources after the CRDs.
// The function returns the three different sets of resources and
// a boolean that tells whether there were any CRDs in the set of
// resources.
func splitAfterCRDs(infos []*resource.Info) (crdSplitResult, bool) {
	var before []*resource.Info
	var after []*resource.Info

	var crds []*resource.Info
	for _, info := range infos {
		if IsCRD(info) {
			crds = append(crds, info)
			continue
		}

		if len(crds) > 0 {
			after = append(after, info)
		} else {
			before = append(before, info)
		}
	}
	return crdSplitResult{
		before: before,
		after:  after,
		crds:   crds,
	}, len(crds) > 0
}

func IsCRD(info *resource.Info) bool {
	gvk, found := toGVK(info)
	if !found {
		return false
	}
	if (gvk.Group == v1.SchemeGroupVersion.Group ||
		gvk.Group == v1beta1.SchemeGroupVersion.Group) &&
		gvk.Kind == "CustomResourceDefinition" {
		return true
	}
	return false
}

func toGVK(info *resource.Info) (schema.GroupVersionKind, bool) {
	if mapping := info.ResourceMapping(); mapping != nil {
		return mapping.GroupVersionKind, true
	}
	if info.Object != nil {
		return info.Object.GetObjectKind().GroupVersionKind(), true
	}
	return schema.GroupVersionKind{}, false
}

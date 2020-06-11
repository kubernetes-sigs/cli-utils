// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package solver

import (
	"reflect"
	"testing"
	"time"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/cmd/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/task"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	applyOptions = &apply.ApplyOptions{}
	pruneOptions = &prune.PruneOptions{}

	depInfo    = createInfo("apps/v1", "Deployment", "foo", "bar")
	customInfo = createInfo("custom.io/v1", "Custom", "Foo", "")
	crdInfo    = createInfo("apiextensions.k8s.io/v1", "CustomResourceDefinition", "CRD", "")
)

func TestTaskQueueSolver_BuildTaskQueue(t *testing.T) {
	testCases := map[string]struct {
		infos         []*resource.Info
		options       Options
		expectedTasks []taskrunner.Task
	}{
		"no resources": {
			infos:   []*resource.Info{},
			options: Options{},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{},
				&task.SendEventTask{},
			},
		},
		"single resource": {
			infos: []*resource.Info{
				depInfo,
			},
			options: Options{},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*resource.Info{
						depInfo,
					},
				},
				&task.SendEventTask{},
			},
		},
		"multiple resources with wait": {
			infos: []*resource.Info{
				depInfo,
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*resource.Info{
						depInfo,
						customInfo,
					},
				},
				&task.SendEventTask{},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						object.InfoToObjMeta(depInfo),
						object.InfoToObjMeta(customInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.SendEventTask{},
			},
		},
		"multiple resources with wait and prune": {
			infos: []*resource.Info{
				depInfo,
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				Prune:            true,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*resource.Info{
						depInfo,
						customInfo,
					},
				},
				&task.SendEventTask{},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						object.InfoToObjMeta(depInfo),
						object.InfoToObjMeta(customInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.SendEventTask{},
				&task.PruneTask{},
				&task.SendEventTask{},
			},
		},
		"multiple resources with wait, prune and dryrun": {
			infos: []*resource.Info{
				depInfo,
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				Prune:            true,
				DryRun:           true,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*resource.Info{
						depInfo,
						customInfo,
					},
				},
				&task.SendEventTask{},
				&task.PruneTask{},
				&task.SendEventTask{},
			},
		},
		"multiple resources including CRD": {
			infos: []*resource.Info{
				crdInfo,
				depInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*resource.Info{
						crdInfo,
					},
				},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						object.InfoToObjMeta(crdInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.ResetRESTMapperTask{},
				&task.ApplyTask{
					Objects: []*resource.Info{
						depInfo,
					},
				},
				&task.SendEventTask{},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						object.InfoToObjMeta(crdInfo),
						object.InfoToObjMeta(depInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.SendEventTask{},
			},
		},
		"no wait with CRDs if it is a dryrun": {
			infos: []*resource.Info{
				crdInfo,
				depInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRun:           true,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*resource.Info{
						crdInfo,
					},
				},
				&task.ApplyTask{
					Objects: []*resource.Info{
						depInfo,
					},
				},
				&task.SendEventTask{},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tqs := TaskQueueSolver{
				ApplyOptions: applyOptions,
				PruneOptions: pruneOptions,
				Mapper:       testutil.NewFakeRESTMapper(),
			}

			tq := tqs.BuildTaskQueue(&fakeResourceObjects{
				infosForApply: tc.infos,
				idsForApply:   object.InfosToObjMetas(tc.infos),
				idsForPrune:   nil,
			}, tc.options)

			tasks := queueToSlice(tq)

			assert.Equal(t, len(tc.expectedTasks), len(tasks))
			for i, expTask := range tc.expectedTasks {
				actualTask := tasks[i]
				assert.Equal(t, getType(expTask), getType(actualTask))

				switch expTsk := expTask.(type) {
				case *task.ApplyTask:
					actApplyTask := toApplyTask(t, actualTask)
					assert.Equal(t, len(expTsk.Objects), len(actApplyTask.Objects))
					for j, obj := range expTsk.Objects {
						actObj := actApplyTask.Objects[j]
						assert.Equal(t, object.InfoToObjMeta(obj), object.InfoToObjMeta(actObj))
					}
				case *taskrunner.WaitTask:
					actWaitTask := toWaitTask(t, actualTask)
					assert.Equal(t, len(expTsk.Identifiers), len(actWaitTask.Identifiers))
					for j, id := range expTsk.Identifiers {
						actID := actWaitTask.Identifiers[j]
						assert.Equal(t, id, actID)
					}
				}
			}
		})
	}
}

func toWaitTask(t *testing.T, task taskrunner.Task) *taskrunner.WaitTask {
	switch tsk := task.(type) {
	case *taskrunner.WaitTask:
		return tsk
	default:
		t.Fatalf("expected type *WaitTask, but got %s", reflect.TypeOf(task).String())
		return nil
	}
}

func toApplyTask(t *testing.T, aTask taskrunner.Task) *task.ApplyTask {
	switch tsk := aTask.(type) {
	case *task.ApplyTask:
		return tsk
	default:
		t.Fatalf("expected type *ApplyTask, but got %s", reflect.TypeOf(aTask).String())
		return nil
	}
}

func createInfo(apiVersion, kind, name, namespace string) *resource.Info {
	return &resource.Info{
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": apiVersion,
				"kind":       kind,
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": namespace,
				},
			},
		},
	}
}

func queueToSlice(tq chan taskrunner.Task) []taskrunner.Task {
	var tasks []taskrunner.Task
	for {
		select {
		case t := <-tq:
			tasks = append(tasks, t)
		default:
			return tasks
		}
	}
}

func getType(task taskrunner.Task) reflect.Type {
	return reflect.TypeOf(task)
}

type fakeResourceObjects struct {
	infosForApply []*resource.Info
	idsForApply   []object.ObjMetadata
	idsForPrune   []object.ObjMetadata
}

func (f *fakeResourceObjects) InfosForApply() []*resource.Info {
	return f.infosForApply
}

func (f *fakeResourceObjects) IdsForApply() []object.ObjMetadata {
	return f.idsForApply
}

func (f *fakeResourceObjects) IdsForPrune() []object.ObjMetadata {
	return f.idsForPrune
}

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
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/task"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	pruneOptions = &prune.PruneOptions{}

	depInfo    = createInfo("apps/v1", "Deployment", "foo", "bar").Object.(*unstructured.Unstructured)
	customInfo = createInfo("custom.io/v1", "Custom", "foo", "").Object.(*unstructured.Unstructured)
	crdInfo    = createInfo("apiextensions.k8s.io/v1", "CustomResourceDefinition", "crd", "").Object.(*unstructured.Unstructured)
)

func TestTaskQueueSolver_BuildTaskQueue(t *testing.T) {
	testCases := map[string]struct {
		objs          []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
	}{
		"no resources": {
			objs:    []*unstructured.Unstructured{},
			options: Options{},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects:  []*unstructured.Unstructured{},
				},
			},
		},
		"single resource": {
			objs: []*unstructured.Unstructured{
				depInfo,
			},
			options: Options{},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						depInfo,
					},
				},
			},
		},
		"multiple resources with wait": {
			objs: []*unstructured.Unstructured{
				depInfo,
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						depInfo,
						customInfo,
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(depInfo),
						ignoreErrInfoToObjMeta(customInfo),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
		},
		"multiple resources with wait and prune": {
			objs: []*unstructured.Unstructured{
				depInfo,
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				Prune:            true,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						depInfo,
						customInfo,
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(depInfo),
						ignoreErrInfoToObjMeta(customInfo),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.PruneTask{
					TaskName: "prune-0",
				},
			},
		},
		"multiple resources with wait, prune and dryrun": {
			objs: []*unstructured.Unstructured{
				depInfo,
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				Prune:            true,
				DryRunStrategy:   common.DryRunClient,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						depInfo,
						customInfo,
					},
				},
				&task.PruneTask{
					TaskName: "prune-0",
				},
			},
		},
		"multiple resources with wait, prune and server-dryrun": {
			objs: []*unstructured.Unstructured{
				depInfo,
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				Prune:            true,
				DryRunStrategy:   common.DryRunServer,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						depInfo,
						customInfo,
					},
				},
				&task.PruneTask{
					TaskName: "prune-0",
				},
			},
		},
		"multiple resources including CRD": {
			objs: []*unstructured.Unstructured{
				crdInfo,
				depInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						crdInfo,
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(crdInfo),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						depInfo,
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(depInfo),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
		},
		"no wait with CRDs if it is a dryrun": {
			objs: []*unstructured.Unstructured{
				crdInfo,
				depInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRunStrategy:   common.DryRunClient,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						crdInfo,
					},
				},
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						depInfo,
					},
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			tqs := TaskQueueSolver{
				PruneOptions: pruneOptions,
				Mapper:       testutil.NewFakeRESTMapper(),
			}

			objs := object.UnstructuredsToObjMetas(tc.objs)
			tq := tqs.BuildTaskQueue(&fakeResourceObjects{
				objsForApply: tc.objs,
				idsForApply:  objs,
				idsForPrune:  nil,
			}, tc.options)

			assert.Equal(t, len(tc.expectedTasks), len(tq.tasks))
			for i, expTask := range tc.expectedTasks {
				actualTask := tq.tasks[i]
				assert.Equal(t, getType(expTask), getType(actualTask))
				assert.Equal(t, expTask.Name(), actualTask.Name())

				switch expTsk := expTask.(type) {
				case *task.ApplyTask:
					actApplyTask := toApplyTask(t, actualTask)
					assert.Equal(t, len(expTsk.Objects), len(actApplyTask.Objects))
					for j, obj := range expTsk.Objects {
						actObj := actApplyTask.Objects[j]
						assert.Equal(t, ignoreErrInfoToObjMeta(obj), ignoreErrInfoToObjMeta(actObj))
					}
				case *taskrunner.WaitTask:
					actWaitTask := toWaitTask(t, actualTask)
					assert.Equal(t, len(expTsk.Ids), len(actWaitTask.Ids))
					for j, id := range expTsk.Ids {
						actID := actWaitTask.Ids[j]
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
		Namespace: namespace,
		Name:      name,
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

func getType(task taskrunner.Task) reflect.Type {
	return reflect.TypeOf(task)
}

type fakeResourceObjects struct {
	objsForApply  []*unstructured.Unstructured
	inventory     inventory.InventoryInfo
	idsForApply   []object.ObjMetadata
	idsForPrune   []object.ObjMetadata
	idsForPrevInv []object.ObjMetadata
}

func (f *fakeResourceObjects) ObjsForApply() []*unstructured.Unstructured {
	return f.objsForApply
}

func (f *fakeResourceObjects) Inventory() inventory.InventoryInfo {
	return f.inventory
}

func (f *fakeResourceObjects) IdsForApply() []object.ObjMetadata {
	return f.idsForApply
}

func (f *fakeResourceObjects) IdsForPrune() []object.ObjMetadata {
	return f.idsForPrune
}

func (f *fakeResourceObjects) IdsForPrevInv() []object.ObjMetadata {
	return f.idsForPrevInv
}

func ignoreErrInfoToObjMeta(info *unstructured.Unstructured) object.ObjMetadata {
	objMeta := object.UnstructuredToObjMeta(info)
	return objMeta
}

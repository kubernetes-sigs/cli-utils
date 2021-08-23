// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package solver

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
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
	resources    = map[string]string{
		"pod": `
kind: Pod
apiVersion: v1
metadata:
  name: test-pod
  namespace: test-namespace
`,
		"default-pod": `
kind: Pod
apiVersion: v1
metadata:
  name: pod-in-default-namespace
  namespace: default
`,
		"deployment": `
kind: Deployment
apiVersion: apps/v1
metadata:
  name: foo
  namespace: test-namespace
  uid: dep-uid
  generation: 1
spec:
  replicas: 1
`,
		"secret": `
kind: Secret
apiVersion: v1
metadata:
  name: secret
  namespace: test-namespace
  uid: secret-uid
  generation: 1
type: Opaque
spec:
  foo: bar
`,
		"namespace": `
kind: Namespace
apiVersion: v1
metadata:
  name: test-namespace
`,

		"crd": `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: crontabs.stable.example.com
spec:
  group: stable.example.com
  versions:
    - name: v1
      served: true
      storage: true
  scope: Namespaced
  names:
    plural: crontabs
    singular: crontab
    kind: CronTab
`,
		"crontab1": `
apiVersion: "stable.example.com/v1"
kind: CronTab
metadata:
  name: cron-tab-01
  namespace: test-namespace
`,
		"crontab2": `
apiVersion: "stable.example.com/v1"
kind: CronTab
metadata:
  name: cron-tab-02
  namespace: test-namespace
`,
	}
)

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "test-inventory-name",
			"namespace": "test-inventory-namespace",
			"labels": map[string]interface{}{
				common.InventoryLabel: "test-inventory-label",
			},
		},
	},
}
var localInv = inventory.WrapInventoryInfoObj(inventoryObj)

func TestTaskQueueBuilder_AppendApplyWaitTasks(t *testing.T) {
	testCases := map[string]struct {
		applyObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		isError       bool
	}{
		"no resources, no tasks": {
			applyObjs:     []*unstructured.Unstructured{},
			expectedTasks: []taskrunner.Task{},
			isError:       false,
		},
		"single resource, one apply task": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"]),
					},
				},
			},
			isError: false,
		},
		"multiple resources with reconcile timeout": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			options: Options{
				ReconcileTimeout: time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"]),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"multiple resources with reconcile timeout and dryrun": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRunStrategy:   common.DryRunClient,
			},
			// No wait task, since it is dry run
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"]),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
			},
			isError: false,
		},
		"multiple resources with reconcile timeout and server-dryrun": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"]),
				testutil.Unstructured(t, resources["default-pod"]),
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRunStrategy:   common.DryRunServer,
			},
			// No wait task, since it is dry run
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["default-pod"]),
					},
				},
			},
			isError: false,
		},
		"multiple resources including CRD": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crd"]),
				testutil.Unstructured(t, resources["crontab2"]),
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"no wait with CRDs if it is a dryrun": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crd"]),
				testutil.Unstructured(t, resources["crontab2"]),
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRunStrategy:   common.DryRunClient,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
				},
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
				},
			},
			isError: false,
		},
		"resources in namespace creates multiple apply tasks": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["pod"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["namespace"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"deployment depends on secret creates multiple tasks": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.Unstructured(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"cyclic dependency returns error": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.Unstructured(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.Unstructured(t, resources["deployment"]))),
			},
			expectedTasks: []taskrunner.Task{},
			isError:       true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			applyIds := object.UnstructuredsToObjMetasOrDie(tc.applyObjs)
			fakeInvClient := inventory.NewFakeInventoryClient(applyIds)
			tqb := TaskQueueBuilder{
				PruneOptions: pruneOptions,
				Mapper:       testutil.NewFakeRESTMapper(),
				InvClient:    fakeInvClient,
			}
			tq, err := tqb.AppendApplyWaitTasks(localInv, tc.applyObjs, tc.options).Build()
			if tc.isError {
				assert.NotNil(t, err, "expected error, but received none")
				return
			}
			assert.Nil(t, err, "unexpected error received")
			assert.Equal(t, len(tc.expectedTasks), len(tq.tasks))
			for i, expTask := range tc.expectedTasks {
				actualTask := tq.tasks[i]
				assert.Equal(t, getType(expTask), getType(actualTask))
				assert.Equal(t, expTask.Name(), actualTask.Name())

				switch expTsk := expTask.(type) {
				case *task.ApplyTask:
					actApplyTask := toApplyTask(t, actualTask)
					assert.Equal(t, len(expTsk.Objects), len(actApplyTask.Objects))
					// Order is NOT important for objects stored within task.
					verifyObjSets(t, expTsk.Objects, actApplyTask.Objects)
				case *taskrunner.WaitTask:
					actWaitTask := toWaitTask(t, actualTask)
					assert.Equal(t, len(expTsk.Ids), len(actWaitTask.Ids))
					// Order is NOT important for ids stored within task.
					if !object.SetEquals(expTsk.Ids, actWaitTask.Ids) {
						t.Errorf("expected wait ids (%v), got (%v)",
							expTsk.Ids, actWaitTask.Ids)
					}
					assert.Equal(t, taskrunner.AllCurrent, actWaitTask.Condition)
				}
			}
		})
	}
}

func TestTaskQueueBuilder_AppendPruneWaitTasks(t *testing.T) {
	testCases := map[string]struct {
		pruneObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		isError       bool
	}{
		"no resources, no tasks": {
			pruneObjs:     []*unstructured.Unstructured{},
			options:       Options{Prune: true},
			expectedTasks: []taskrunner.Task{},
			isError:       false,
		},
		"single resource, one prune task": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["default-pod"]),
			},
			options: Options{Prune: true},
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["default-pod"]),
					},
				},
			},
			isError: false,
		},
		"multiple resources, one prune task": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["default-pod"]),
				testutil.Unstructured(t, resources["pod"]),
			},
			options: Options{Prune: true},
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["default-pod"]),
						testutil.Unstructured(t, resources["pod"]),
					},
				},
			},
			isError: false,
		},
		"dependent resources, two prune tasks, two wait tasks": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t, testutil.Unstructured(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			options: Options{Prune: true},
			// Opposite ordering when pruning/deleting
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["pod"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"multiple resources with prune timeout and server-dryrun": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"]),
				testutil.Unstructured(t, resources["default-pod"]),
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRunStrategy:   common.DryRunServer,
				Prune:            true,
			},
			// No wait task, since it is dry run
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["default-pod"]),
					},
				},
			},
			isError: false,
		},
		"multiple resources including CRD": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crd"]),
				testutil.Unstructured(t, resources["crontab2"]),
			},
			options: Options{Prune: true},
			// Opposite ordering when pruning/deleting.
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"no wait with CRDs if it is a dryrun": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["crontab1"]),
				testutil.Unstructured(t, resources["crd"]),
				testutil.Unstructured(t, resources["crontab2"]),
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRunStrategy:   common.DryRunClient,
				Prune:            true,
			},
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
				},
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
				},
			},
			isError: false,
		},
		"resources in namespace creates multiple apply tasks": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["namespace"]),
				testutil.Unstructured(t, resources["pod"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			options: Options{Prune: true},
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["namespace"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"cyclic dependency returns error": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.Unstructured(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.Unstructured(t, resources["deployment"]))),
			},
			options:       Options{Prune: true},
			expectedTasks: []taskrunner.Task{},
			isError:       true,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			pruneIds := object.UnstructuredsToObjMetasOrDie(tc.pruneObjs)
			fakeInvClient := inventory.NewFakeInventoryClient(pruneIds)
			tqb := TaskQueueBuilder{
				PruneOptions: pruneOptions,
				Mapper:       testutil.NewFakeRESTMapper(),
				InvClient:    fakeInvClient,
			}
			emptyPruneFilters := []filter.ValidationFilter{}
			tq, err := tqb.AppendPruneWaitTasks(tc.pruneObjs, emptyPruneFilters, tc.options).Build()
			if tc.isError {
				assert.NotNil(t, err, "expected error, but received none")
				return
			}
			assert.Nil(t, err, "unexpected error received")
			assert.Equal(t, len(tc.expectedTasks), len(tq.tasks))
			for i, expTask := range tc.expectedTasks {
				actualTask := tq.tasks[i]
				assert.Equal(t, getType(expTask), getType(actualTask))
				assert.Equal(t, expTask.Name(), actualTask.Name())

				switch expTsk := expTask.(type) {
				case *task.PruneTask:
					actPruneTask := toPruneTask(t, actualTask)
					assert.Equal(t, len(expTsk.Objects), len(actPruneTask.Objects))
					verifyObjSets(t, expTsk.Objects, actPruneTask.Objects)
				case *taskrunner.WaitTask:
					actWaitTask := toWaitTask(t, actualTask)
					assert.Equal(t, len(expTsk.Ids), len(actWaitTask.Ids))
					if !object.SetEquals(expTsk.Ids, actWaitTask.Ids) {
						t.Errorf("expected wait ids (%v), got (%v)",
							expTsk.Ids, actWaitTask.Ids)
					}
					assert.Equal(t, taskrunner.AllNotFound, actWaitTask.Condition)
				}
			}
		})
	}
}

// verifyObjSets ensures the slice of expected objects is the same as
// the actual slice of objects. Order is NOT important.
func verifyObjSets(t *testing.T, expected []*unstructured.Unstructured, actual []*unstructured.Unstructured) {
	if len(expected) != len(actual) {
		t.Fatalf("expected set size (%d), got (%d)", len(expected), len(actual))
	}
	for _, obj := range expected {
		if !containsObj(actual, obj) {
			t.Fatalf("expected object (%v) not in found", obj)
		}
	}
}

// containsObj returns true if the passed object is within the passed
// slice of objects; false otherwise.
func containsObj(objs []*unstructured.Unstructured, obj *unstructured.Unstructured) bool {
	ids := object.UnstructuredsToObjMetasOrDie(objs)
	id := object.UnstructuredToObjMetaOrDie(obj)
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
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

func toPruneTask(t *testing.T, pTask taskrunner.Task) *task.PruneTask {
	switch tsk := pTask.(type) {
	case *task.PruneTask:
		return tsk
	default:
		t.Fatalf("expected type *PruneTask, but got %s", reflect.TypeOf(pTask).String())
		return nil
	}
}

func getType(task taskrunner.Task) reflect.Type {
	return reflect.TypeOf(task)
}

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
	"sigs.k8s.io/cli-utils/pkg/apply/mutator"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/cli-utils/pkg/apply/task"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/graph"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var (
	pruner    = &prune.Pruner{}
	resources = map[string]string{
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

func TestTaskQueueBuilder_AppendApplyWaitTasks(t *testing.T) {
	testCases := map[string]struct {
		applyObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		expectedError error
	}{
		"no resources, no tasks": {
			applyObjs:     []*unstructured.Unstructured{},
			expectedTasks: []taskrunner.Task{},
		},
		"single resource, one apply task, one wait task": {
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
				taskrunner.NewWaitTask(
					"wait-0",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"multiple resource with no timeout": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
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
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
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
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
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
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
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
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"deployment depends on secret creates multiple tasks": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
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
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"],
							testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"cyclic dependency returns error": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
			},
			expectedTasks: []taskrunner.Task{},
			expectedError: validation.NewError(
				graph.CyclicDependencyError{
					Edges: []graph.Edge{
						{
							From: testutil.ToIdentifier(t, resources["secret"]),
							To:   testutil.ToIdentifier(t, resources["deployment"]),
						},
						{
							From: testutil.ToIdentifier(t, resources["deployment"]),
							To:   testutil.ToIdentifier(t, resources["secret"]),
						},
					},
				},
				testutil.ToIdentifier(t, resources["secret"]),
				testutil.ToIdentifier(t, resources["deployment"]),
			),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			applyIds := object.UnstructuredSetToObjMetadataSet(tc.applyObjs)
			fakeInvClient := inventory.NewFakeInventoryClient(applyIds)
			tqb := TaskQueueBuilder{
				Pruner:    pruner,
				Mapper:    testutil.NewFakeRESTMapper(),
				InvClient: fakeInvClient,
			}
			tq, err := tqb.AppendApplyWaitTasks(
				tc.applyObjs,
				[]filter.ValidationFilter{},
				[]mutator.Interface{},
				tc.options,
			).Build()
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedTasks), len(tq.tasks))
			for i, expTask := range tc.expectedTasks {
				actualTask := tq.tasks[i]
				assert.Equal(t, getType(expTask), getType(actualTask))
				assert.Equal(t, expTask.Name(), actualTask.Name())

				switch expTsk := expTask.(type) {
				case *task.ApplyTask:
					actApplyTask := toApplyTask(t, actualTask)
					testutil.AssertEqual(t, expTsk.Objects, actApplyTask.Objects, "ApplyTask mismatch")
				case *taskrunner.WaitTask:
					actWaitTask := toWaitTask(t, actualTask)
					testutil.AssertEqual(t, expTsk.Ids, actWaitTask.Ids)
					assert.Equal(t, taskrunner.AllCurrent, actWaitTask.Condition, "WaitTask mismatch")
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
		expectedError error
	}{
		"no resources, no tasks": {
			pruneObjs:     []*unstructured.Unstructured{},
			options:       Options{Prune: true},
			expectedTasks: []taskrunner.Task{},
		},
		"single resource, one prune task, one wait task": {
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
				taskrunner.NewWaitTask(
					"wait-0",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["default-pod"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"multiple resources, one prune task, one wait task": {
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
				taskrunner.NewWaitTask(
					"wait-0",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["default-pod"]),
						testutil.ToIdentifier(t, resources["pod"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"dependent resources, two prune tasks, two wait tasks": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"]),
			},
			options: Options{Prune: true},
			// Opposite ordering when pruning/deleting
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"],
							testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"single resource with prune timeout has wait task": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"]),
			},
			options: Options{
				Prune:        true,
				PruneTimeout: 3 * time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.PruneTask{
					TaskName: "prune-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
					},
					taskrunner.AllCurrent,
					3*time.Minute,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"multiple resources with prune timeout and server-dryrun": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["pod"]),
				testutil.Unstructured(t, resources["default-pod"]),
			},
			options: Options{
				PruneTimeout:   time.Minute,
				DryRunStrategy: common.DryRunServer,
				Prune:          true,
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
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
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
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["namespace"]),
					},
				},
				taskrunner.NewWaitTask(
					"wait-1",
					object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
		},
		"cyclic dependency returns error": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
			},
			options:       Options{Prune: true},
			expectedTasks: []taskrunner.Task{},
			expectedError: validation.NewError(
				graph.CyclicDependencyError{
					Edges: []graph.Edge{
						{
							From: testutil.ToIdentifier(t, resources["secret"]),
							To:   testutil.ToIdentifier(t, resources["deployment"]),
						},
						{
							From: testutil.ToIdentifier(t, resources["deployment"]),
							To:   testutil.ToIdentifier(t, resources["secret"]),
						},
					},
				},
				testutil.ToIdentifier(t, resources["secret"]),
				testutil.ToIdentifier(t, resources["deployment"]),
			),
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			pruneIds := object.UnstructuredSetToObjMetadataSet(tc.pruneObjs)
			fakeInvClient := inventory.NewFakeInventoryClient(pruneIds)
			tqb := TaskQueueBuilder{
				Pruner:    pruner,
				Mapper:    testutil.NewFakeRESTMapper(),
				InvClient: fakeInvClient,
			}
			emptyPruneFilters := []filter.ValidationFilter{}
			tq, err := tqb.AppendPruneWaitTasks(tc.pruneObjs, emptyPruneFilters, tc.options).Build()
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, len(tc.expectedTasks), len(tq.tasks))
			for i, expTask := range tc.expectedTasks {
				actualTask := tq.tasks[i]
				assert.Equal(t, getType(expTask), getType(actualTask))
				assert.Equal(t, expTask.Name(), actualTask.Name())

				switch expTsk := expTask.(type) {
				case *task.PruneTask:
					actPruneTask := toPruneTask(t, actualTask)
					testutil.AssertEqual(t, expTsk.Objects, actPruneTask.Objects, "PruneTask mismatch")
				case *taskrunner.WaitTask:
					actWaitTask := toWaitTask(t, actualTask)
					testutil.AssertEqual(t, expTsk.Ids, actWaitTask.Ids)
					assert.Equal(t, taskrunner.AllNotFound, actWaitTask.Condition, "WaitTask mismatch")
					// Validate the prune wait timeout.
					assert.Equal(t, tc.options.PruneTimeout, actualTask.(*taskrunner.WaitTask).Timeout)
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

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package solver

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
	// Use a custom Asserter to customize the comparison options
	asserter := testutil.NewAsserter(
		cmpopts.EquateErrors(),
		waitTaskComparer(),
	)

	testCases := map[string]struct {
		applyObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		expectedError error
	}{
		"no resources, no tasks": {
			applyObjs:     []*unstructured.Unstructured{},
			expectedTasks: nil,
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
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					Condition: taskrunner.AllCurrent,
				},
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
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					Condition: taskrunner.AllCurrent,
				},
			},
		},
		"multiple resources with reconcile timeout": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			options: Options{
				ReconcileTimeout: 1 * time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["secret"]),
						testutil.Unstructured(t, resources["deployment"]),
					},
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["secret"]),
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					Condition: taskrunner.AllCurrent,
					Timeout:   1 * time.Minute,
				},
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
					DryRunStrategy: common.DryRunClient,
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
					DryRunStrategy: common.DryRunServer,
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
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					Condition: taskrunner.AllCurrent,
				},
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-1",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					Condition: taskrunner.AllCurrent,
				},
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
					DryRunStrategy: common.DryRunClient,
				},
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
					DryRunStrategy: common.DryRunClient,
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
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					Condition: taskrunner.AllCurrent,
				},
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["secret"]),
						testutil.Unstructured(t, resources["pod"]),
					},
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-1",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["secret"]),
						testutil.ToIdentifier(t, resources["pod"]),
					},
					Condition: taskrunner.AllCurrent,
				},
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
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					Condition: taskrunner.AllCurrent,
				},
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"],
							testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
					},
					DryRunStrategy: common.DryRunNone,
				},
				&taskrunner.WaitTask{
					TaskName: "wait-1",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					Condition: taskrunner.AllCurrent,
				},
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
			mapper := testutil.NewFakeRESTMapper()
			// inject mapper for equality comparison
			for _, t := range tc.expectedTasks {
				switch typedTask := t.(type) {
				case *task.ApplyTask:
					typedTask.Mapper = mapper
				case *taskrunner.WaitTask:
					typedTask.Mapper = mapper
				}
			}

			applyIds := object.UnstructuredSetToObjMetadataSet(tc.applyObjs)
			fakeInvClient := inventory.NewFakeInventoryClient(applyIds)
			vCollector := &validation.Collector{}
			tqb := TaskQueueBuilder{
				Pruner:    pruner,
				Mapper:    mapper,
				InvClient: fakeInvClient,
				Collector: vCollector,
			}
			var filters []filter.ValidationFilter
			var mutators []mutator.Interface
			tq := tqb.AppendApplyWaitTasks(
				tc.applyObjs,
				filters,
				mutators,
				tc.options,
			).Build()
			err := vCollector.ToError()
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			asserter.Equal(t, tc.expectedTasks, tq.tasks)
		})
	}
}

func TestTaskQueueBuilder_AppendPruneWaitTasks(t *testing.T) {
	// Use a custom Asserter to customize the comparison options
	asserter := testutil.NewAsserter(
		cmpopts.EquateErrors(),
		waitTaskComparer(),
	)

	testCases := map[string]struct {
		pruneObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		expectedError error
	}{
		"no resources, no tasks": {
			pruneObjs:     []*unstructured.Unstructured{},
			options:       Options{Prune: true},
			expectedTasks: nil,
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
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["default-pod"]),
					},
					Condition: taskrunner.AllNotFound,
				},
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
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["default-pod"]),
						testutil.ToIdentifier(t, resources["pod"]),
					},
					Condition: taskrunner.AllNotFound,
				},
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
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
					},
					Condition: taskrunner.AllNotFound,
				},
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["secret"]),
					},
				},
				&taskrunner.WaitTask{
					TaskName: "wait-1",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					Condition: taskrunner.AllNotFound,
				},
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
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
					},
					Condition: taskrunner.AllNotFound,
					Timeout:   3 * time.Minute,
				},
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
					DryRunStrategy: common.DryRunServer,
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
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					Condition: taskrunner.AllNotFound,
				},
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
				},
				&taskrunner.WaitTask{
					TaskName: "wait-1",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					Condition: taskrunner.AllNotFound,
				},
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
					DryRunStrategy: common.DryRunClient,
				},
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
					DryRunStrategy: common.DryRunClient,
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
				&taskrunner.WaitTask{
					TaskName: "wait-0",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					Condition: taskrunner.AllNotFound,
				},
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["namespace"]),
					},
				},
				&taskrunner.WaitTask{
					TaskName: "wait-1",
					Ids: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					Condition: taskrunner.AllNotFound,
				},
			},
		},
		"cyclic dependency": {
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
		"cyclic dependency and valid": {
			pruneObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
				testutil.Unstructured(t, resources["pod"]),
			},
			options: Options{Prune: true},
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
					taskrunner.AllCurrent, 1*time.Second,
					testutil.NewFakeRESTMapper(),
				),
			},
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
			mapper := testutil.NewFakeRESTMapper()
			// inject mapper & pruner for equality comparison
			for _, t := range tc.expectedTasks {
				switch typedTask := t.(type) {
				case *task.PruneTask:
					typedTask.Pruner = &prune.Pruner{}
				case *taskrunner.WaitTask:
					typedTask.Mapper = mapper
				}
			}

			pruneIds := object.UnstructuredSetToObjMetadataSet(tc.pruneObjs)
			fakeInvClient := inventory.NewFakeInventoryClient(pruneIds)
			vCollector := &validation.Collector{}
			tqb := TaskQueueBuilder{
				Pruner:    pruner,
				Mapper:    mapper,
				InvClient: fakeInvClient,
				Collector: vCollector,
			}
			var emptyPruneFilters []filter.ValidationFilter
			tq := tqb.AppendPruneWaitTasks(tc.pruneObjs, emptyPruneFilters, tc.options).Build()
			err := vCollector.ToError()
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				return
			}
			assert.NoError(t, err)
			asserter.Equal(t, tc.expectedTasks, tq.tasks)
		})
	}
}

// waitTaskComparer allows comparion of WaitTasks, ignoring private fields.
func waitTaskComparer() cmp.Option {
	return cmp.Comparer(func(x, y *taskrunner.WaitTask) bool {
		if x == nil {
			return y == nil
		}
		if y == nil {
			return false
		}
		return x.TaskName == y.TaskName &&
			x.Ids.Hash() == y.Ids.Hash() && // exact order match
			x.Condition == y.Condition &&
			x.Timeout == y.Timeout &&
			cmp.Equal(x.Mapper, y.Mapper)
	})
}

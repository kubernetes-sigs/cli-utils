// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package solver

import (
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

func TestTaskQueueBuilder_AppendApplyWaitTasks(t *testing.T) {
	mapper := testutil.NewFakeRESTMapper()

	testCases := map[string]struct {
		applyObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		isError       bool
	}{
		"no resources, no tasks": {
			applyObjs:     []*unstructured.Unstructured{},
			expectedTasks: []taskrunner.Task(nil),
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
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
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
				ReconcileTimeout: 2 * time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"]),
						testutil.Unstructured(t, resources["secret"]),
					},
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent,
					2*time.Minute,
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
				ReconcileTimeout: 2 * time.Minute,
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
					Mapper:         mapper,
					DryRunStrategy: common.DryRunClient,
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
				ReconcileTimeout: 2 * time.Minute,
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
					Mapper:         mapper,
					DryRunStrategy: common.DryRunServer,
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
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
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
				ReconcileTimeout: 1 * time.Minute,
				DryRunStrategy:   common.DryRunClient,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
					Mapper:         mapper,
					DryRunStrategy: common.DryRunClient,
				},
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
					Mapper:         mapper,
					DryRunStrategy: common.DryRunClient,
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
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["secret"]),
					},
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
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
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"],
							testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
					},
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"external dependency creates wait task": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
			},
			expectedTasks: []taskrunner.Task{
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"],
							testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
					},
					Mapper:         mapper,
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
					taskrunner.AllCurrent,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
		},
		"cyclic dependency returns error": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
				testutil.Unstructured(t, resources["secret"],
					testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["deployment"]))),
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
				Mapper:       mapper,
				InvClient:    fakeInvClient,
			}
			filters := []filter.ValidationFilter(nil)
			mutators := []mutator.Interface(nil)
			tq, err := tqb.AppendApplyWaitTasks(tc.applyObjs, filters, mutators, tc.options).Build()
			if tc.isError {
				assert.NotNil(t, err, "expected error, but received none")
				return
			}
			assert.Nil(t, err, "unexpected error received")
			testutil.AssertEqual(t, tq.tasks, tc.expectedTasks)
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
			expectedTasks: []taskrunner.Task(nil),
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
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
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
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
				},
			},
			isError: false,
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
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["pod"]),
					},
					taskrunner.AllNotFound,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["secret"]),
					},
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllNotFound,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
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
					PruneOptions: &prune.PruneOptions{},
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["pod"]),
					},
					taskrunner.AllNotFound,
					3*time.Minute,
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
				ReconcileTimeout: 1 * time.Minute,
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
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunServer,
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
						testutil.Unstructured(t, resources["crontab2"]),
						testutil.Unstructured(t, resources["crontab1"]),
					},
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					taskrunner.AllNotFound,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["crd"]),
					},
					taskrunner.AllNotFound,
					1*time.Minute,
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
				ReconcileTimeout: 1 * time.Minute,
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
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunClient,
				},
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["crd"]),
					},
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunClient,
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
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-0",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					taskrunner.AllNotFound,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
				&task.PruneTask{
					TaskName: "prune-1",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["namespace"]),
					},
					PruneOptions:   &prune.PruneOptions{},
					DryRunStrategy: common.DryRunNone,
				},
				taskrunner.NewWaitTask(
					"wait-1",
					[]object.ObjMetadata{
						testutil.ToIdentifier(t, resources["namespace"]),
					},
					taskrunner.AllNotFound,
					1*time.Minute,
					testutil.NewFakeRESTMapper()),
			},
			isError: false,
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
			emptyPruneFilters := []filter.ValidationFilter(nil)
			tq, err := tqb.AppendPruneWaitTasks(tc.pruneObjs, emptyPruneFilters, tc.options).Build()
			if tc.isError {
				assert.NotNil(t, err, "expected error, but received none")
				return
			}
			assert.Nil(t, err, "unexpected error received")
			testutil.AssertEqual(t, tq.tasks, tc.expectedTasks)
		})
	}
}

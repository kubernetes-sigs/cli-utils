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

func newInvObject(name, namespace, inventoryID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					common.InventoryLabel: inventoryID,
				},
			},
			"data": map[string]string{},
		},
	}
}

func TestTaskQueueBuilder_ApplyBuild(t *testing.T) {
	// Use a custom Asserter to customize the comparison options
	asserter := testutil.NewAsserter(
		cmpopts.EquateErrors(),
		waitTaskComparer(),
		fakeClientComparer(),
		inventoryInfoComparer(),
	)

	invInfo := inventory.WrapInventoryInfoObj(newInvObject(
		"abc-123", "default", "test"))

	testCases := map[string]struct {
		applyObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		expectedError error
	}{
		"no resources, no apply or wait tasks": {
			applyObjs: []*unstructured.Unstructured{},
			expectedTasks: []taskrunner.Task{
				&task.InvSetTask{
					TaskName:      "inventory-set-0",
					InvClient:     &inventory.FakeClient{},
					InvInfo:       invInfo,
					PrevInventory: object.ObjMetadataSet{},
				},
			},
		},
		"single resource, one apply task, one wait task": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
			},
			expectedTasks: []taskrunner.Task{
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["deployment"]),
					},
				},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
					},
				},
			},
		},
		"multiple resource with no timeout": {
			applyObjs: []*unstructured.Unstructured{
				testutil.Unstructured(t, resources["deployment"]),
				testutil.Unstructured(t, resources["secret"]),
			},
			expectedTasks: []taskrunner.Task{
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["deployment"]),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
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
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["secret"]),
						testutil.Unstructured(t, resources["deployment"]),
					},
				},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["secret"]),
						testutil.ToIdentifier(t, resources["deployment"]),
					},
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
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["deployment"]),
						testutil.Unstructured(t, resources["secret"]),
					},
					DryRun: common.DryRunClient,
				},
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["deployment"]),
						testutil.Unstructured(t, resources["secret"]),
					},
					DryRunStrategy: common.DryRunClient,
				},
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
					DryRun: common.DryRunClient,
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
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["default-pod"]),
					},
					DryRun: common.DryRunServer,
				},
				&task.ApplyTask{
					TaskName: "apply-0",
					Objects: []*unstructured.Unstructured{
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["default-pod"]),
					},
					DryRunStrategy: common.DryRunServer,
				},
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["default-pod"]),
					},
					DryRun: common.DryRunServer,
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
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crd"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
				},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crd"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
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
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["crontab1"]),
						testutil.Unstructured(t, resources["crd"]),
						testutil.Unstructured(t, resources["crontab2"]),
					},
					DryRun: common.DryRunClient,
				},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crd"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					DryRun: common.DryRunClient,
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
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["namespace"]),
						testutil.Unstructured(t, resources["pod"]),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["namespace"]),
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
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
				&task.InvAddTask{
					TaskName:  "inventory-add-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					Objects: object.UnstructuredSet{
						testutil.Unstructured(t, resources["deployment"],
							testutil.AddDependsOn(t, testutil.ToIdentifier(t, resources["secret"]))),
						testutil.Unstructured(t, resources["secret"]),
					},
				},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["deployment"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
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
			fakeInvClient := inventory.NewFakeClient(applyIds)
			vCollector := &validation.Collector{}
			tqb := TaskQueueBuilder{
				Pruner:    pruner,
				Mapper:    mapper,
				InvClient: fakeInvClient,
				Collector: vCollector,
			}
			tq := tqb.WithInventory(invInfo).
				WithApplyObjects(tc.applyObjs).
				Build(tc.options)
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

func TestTaskQueueBuilder_PruneBuild(t *testing.T) {
	// Use a custom Asserter to customize the comparison options
	asserter := testutil.NewAsserter(
		cmpopts.EquateErrors(),
		waitTaskComparer(),
		fakeClientComparer(),
		inventoryInfoComparer(),
	)

	invInfo := inventory.WrapInventoryInfoObj(newInvObject(
		"abc-123", "default", "test"))

	testCases := map[string]struct {
		pruneObjs     []*unstructured.Unstructured
		options       Options
		expectedTasks []taskrunner.Task
		expectedError error
	}{
		"no resources, no apply or prune tasks": {
			pruneObjs: []*unstructured.Unstructured{},
			options:   Options{Prune: true},
			expectedTasks: []taskrunner.Task{
				&task.InvSetTask{
					TaskName:      "inventory-set-0",
					InvClient:     &inventory.FakeClient{},
					InvInfo:       invInfo,
					PrevInventory: object.ObjMetadataSet{},
				},
			},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["default-pod"]),
					},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["default-pod"]),
						testutil.ToIdentifier(t, resources["pod"]),
					},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
					},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["default-pod"]),
					},
					DryRun: common.DryRunServer,
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crd"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["crontab1"]),
						testutil.ToIdentifier(t, resources["crd"]),
						testutil.ToIdentifier(t, resources["crontab2"]),
					},
					DryRun: common.DryRunClient,
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["namespace"]),
						testutil.ToIdentifier(t, resources["pod"]),
						testutil.ToIdentifier(t, resources["secret"]),
					},
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
				&task.InvSetTask{
					TaskName:  "inventory-set-0",
					InvClient: &inventory.FakeClient{},
					InvInfo:   invInfo,
					PrevInventory: object.ObjMetadataSet{
						testutil.ToIdentifier(t, resources["pod"]),
					},
				},
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
			fakeInvClient := inventory.NewFakeClient(pruneIds)
			vCollector := &validation.Collector{}
			tqb := TaskQueueBuilder{
				Pruner:    pruner,
				Mapper:    mapper,
				InvClient: fakeInvClient,
				Collector: vCollector,
			}
			tq := tqb.WithInventory(invInfo).
				WithPruneObjects(tc.pruneObjs).
				Build(tc.options)
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

// fakeClientComparer allows comparion of inventory.FakeClient, ignoring objs.
func fakeClientComparer() cmp.Option {
	return cmp.Comparer(func(x, y *inventory.FakeClient) bool {
		if x == nil {
			return y == nil
		}
		if y == nil {
			return false
		}
		return true
	})
}

// inventoryInfoComparer allows comparion of inventory.Info, ignoring impl.
func inventoryInfoComparer() cmp.Option {
	return cmp.Comparer(func(x, y inventory.Info) bool {
		return x.ID() == y.ID() &&
			x.Name() == y.Name() &&
			x.Namespace() == y.Namespace() &&
			x.Strategy() == y.Strategy()
	})
}

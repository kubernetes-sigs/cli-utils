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
	"sigs.k8s.io/cli-utils/pkg/object/graph"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

const (
	crdGroup      = "stable.example.com"
	crdKind       = "CronTab"
	crdKindPlural = "crontabs"
	crdName       = crdKindPlural + "." + crdGroup
	namespace     = "test-namespace"
)

var (
	pruneOptions = &prune.PruneOptions{}

	depInfo       = createInfo("apps/v1", "Deployment", "test-deployment", namespace).Object.(*unstructured.Unstructured)
	namespaceInfo = createInfo("v1", "Namespace", namespace, "").Object.(*unstructured.Unstructured)
	customInfo    = createInfo(crdGroup+"/v1", crdKind, "my-cron-object", "").Object.(*unstructured.Unstructured)
	crInfo        = createInfo(crdGroup+"/v1", crdKind, "second-cron-object", "").Object.(*unstructured.Unstructured)
	crdInfo       = createInfo("apiextensions.k8s.io/v1", "CustomResourceDefinition", crdName, "").Object.(*unstructured.Unstructured)
	cmInfo        = createInfo("v1", "ConfigMap", "test-cm", namespace).Object.(*unstructured.Unstructured)
)

func createCRD(obj *unstructured.Unstructured) *unstructured.Unstructured {
	_ = unstructured.SetNestedField(obj.Object, crdGroup, "spec", "group")
	_ = unstructured.SetNestedField(obj.Object, crdKind, "spec", "names", "kind")
	return obj
}

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
				&task.ApplyTask{},
				&task.SendEventTask{},
			},
		},
		"single resource": {
			objs: []*unstructured.Unstructured{
				depInfo,
			},
			options: Options{},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*unstructured.Unstructured{
						depInfo,
					},
				},
				&task.SendEventTask{},
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
					Objects: []*unstructured.Unstructured{
						depInfo,
						customInfo,
					},
				},
				&task.SendEventTask{},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(depInfo),
						ignoreErrInfoToObjMeta(customInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.SendEventTask{},
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
					Objects: []*unstructured.Unstructured{
						depInfo,
						customInfo,
					},
				},
				&task.SendEventTask{},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(depInfo),
						ignoreErrInfoToObjMeta(customInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.SendEventTask{},
				&task.PruneTask{},
				&task.SendEventTask{},
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
					Objects: []*unstructured.Unstructured{
						depInfo,
						customInfo,
					},
				},
				&task.SendEventTask{},
				&task.PruneTask{},
				&task.SendEventTask{},
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
					Objects: []*unstructured.Unstructured{
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
			objs: []*unstructured.Unstructured{
				createCRD(crdInfo),
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*unstructured.Unstructured{
						createCRD(crdInfo),
					},
				},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(crdInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.ResetRESTMapperTask{},
				&task.ApplyTask{
					Objects: []*unstructured.Unstructured{
						customInfo,
					},
				},
				&task.SendEventTask{},
				taskrunner.NewWaitTask(
					[]object.ObjMetadata{
						ignoreErrInfoToObjMeta(customInfo),
					},
					taskrunner.AllCurrent, 1*time.Second),
				&task.SendEventTask{},
			},
		},
		"no wait with CRDs if it is a dryrun": {
			objs: []*unstructured.Unstructured{
				createCRD(crdInfo),
				customInfo,
			},
			options: Options{
				ReconcileTimeout: time.Minute,
				DryRunStrategy:   common.DryRunClient,
			},
			expectedTasks: []taskrunner.Task{
				&task.ApplyTask{
					Objects: []*unstructured.Unstructured{
						createCRD(crdInfo),
					},
				},
				&task.ApplyTask{
					Objects: []*unstructured.Unstructured{
						customInfo,
					},
				},
				&task.SendEventTask{},
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

			tasks := queueToSlice(tq)

			assert.Equal(t, len(tc.expectedTasks), len(tasks))
			for i, expTask := range tc.expectedTasks {
				actualTask := tasks[i]
				assert.Equal(t, getType(expTask), getType(actualTask))

				switch expTsk := expTask.(type) {
				case *task.ApplyTask:
					actApplyTask := toApplyTask(t, actualTask)
					assert.Equal(t, len(expTsk.Objects), len(actApplyTask.Objects))
					expectedObjs := object.UnstructuredsToObjMetas(expTsk.Objects)
					actualObjs := object.UnstructuredsToObjMetas(actApplyTask.Objects)
					if !object.SetEquals(expectedObjs, actualObjs) {
						t.Errorf("expected apply objs (%s), got apply objs (%s)\n", expectedObjs, actualObjs)
					}
				case *taskrunner.WaitTask:
					actWaitTask := toWaitTask(t, actualTask)
					expectedObjs := expTsk.Identifiers
					actualObjs := actWaitTask.Identifiers
					assert.Equal(t, len(expectedObjs), len(actualObjs))
					if !object.SetEquals(expectedObjs, actualObjs) {
						t.Errorf("expected wait objs (%s), got wait objs (%s)\n", expectedObjs, actualObjs)
					}
				}
			}
		})
	}
}

func TestAddNamespaceEdges(t *testing.T) {
	testCases := map[string]struct {
		objs     []*unstructured.Unstructured
		expected []graph.Edge
	}{
		"no objects means no edges added to graph": {
			objs:     []*unstructured.Unstructured{},
			expected: []graph.Edge{},
		},
		"one object means no edges added to graph": {
			objs:     []*unstructured.Unstructured{depInfo},
			expected: []graph.Edge{},
		},
		"two unrelated objects means no edges added to graph": {
			objs:     []*unstructured.Unstructured{depInfo, customInfo},
			expected: []graph.Edge{},
		},
		"one namespace and one object NOT in namespace is zero edges": {
			objs:     []*unstructured.Unstructured{customInfo, namespaceInfo},
			expected: []graph.Edge{},
		},
		"one namespace and one object in namespace is one edge": {
			objs: []*unstructured.Unstructured{depInfo, namespaceInfo},
			expected: []graph.Edge{
				{
					From: ignoreErrInfoToObjMeta(depInfo),
					To:   ignoreErrInfoToObjMeta(namespaceInfo),
				},
			},
		},
		"one namespace and one object in namespace and one not is one edge": {
			objs: []*unstructured.Unstructured{depInfo, namespaceInfo, customInfo},
			expected: []graph.Edge{
				{
					From: ignoreErrInfoToObjMeta(depInfo),
					To:   ignoreErrInfoToObjMeta(namespaceInfo),
				},
			},
		},
		"one namespace and two object in namespace is two edges": {
			objs: []*unstructured.Unstructured{depInfo, namespaceInfo, cmInfo},
			expected: []graph.Edge{
				{
					From: ignoreErrInfoToObjMeta(depInfo),
					To:   ignoreErrInfoToObjMeta(namespaceInfo),
				},
				{
					From: ignoreErrInfoToObjMeta(cmInfo),
					To:   ignoreErrInfoToObjMeta(namespaceInfo),
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g := graph.New()
			addNamespaceEdges(g, tc.objs)
			actual := g.GetEdges()
			if len(tc.expected) != len(actual) {
				t.Errorf("expected num edges %d, got %d\n", len(tc.expected), len(actual))
			}
			for _, expectedEdge := range tc.expected {
				found := false
				for _, actualEdge := range actual {
					if expectedEdge == actualEdge {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected edge (%s) not found\n", expectedEdge)
				}
			}
		})
	}
}

func TestAddCRDEdges(t *testing.T) {
	testCases := map[string]struct {
		objs     []*unstructured.Unstructured
		expected []graph.Edge
	}{
		"no objects means no edges added to graph": {
			objs:     []*unstructured.Unstructured{},
			expected: []graph.Edge{},
		},
		"one object means no edges added to graph": {
			objs:     []*unstructured.Unstructured{depInfo},
			expected: []graph.Edge{},
		},
		"two unrelated objects means no edges added to graph": {
			objs:     []*unstructured.Unstructured{depInfo, customInfo},
			expected: []graph.Edge{},
		},
		"one CRD and one object NOT custom resource is zero edges": {
			objs:     []*unstructured.Unstructured{crdInfo, depInfo},
			expected: []graph.Edge{},
		},
		"one CRD and one custom resource is one edge": {
			objs: []*unstructured.Unstructured{crdInfo, customInfo},
			expected: []graph.Edge{
				{
					From: ignoreErrInfoToObjMeta(customInfo),
					To:   ignoreErrInfoToObjMeta(crdInfo),
				},
			},
		},
		"one CRD and one custom resource and one not is one edge": {
			objs: []*unstructured.Unstructured{crdInfo, customInfo, cmInfo},
			expected: []graph.Edge{
				{
					From: ignoreErrInfoToObjMeta(customInfo),
					To:   ignoreErrInfoToObjMeta(crdInfo),
				},
			},
		},
		"one CRD and two custom resources is two edges": {
			objs: []*unstructured.Unstructured{crdInfo, customInfo, crInfo},
			expected: []graph.Edge{
				{
					From: ignoreErrInfoToObjMeta(customInfo),
					To:   ignoreErrInfoToObjMeta(crdInfo),
				},
				{
					From: ignoreErrInfoToObjMeta(crInfo),
					To:   ignoreErrInfoToObjMeta(crdInfo),
				},
			},
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			g := graph.New()
			addCRDEdges(g, tc.objs)
			actual := g.GetEdges()
			if len(tc.expected) != len(actual) {
				t.Errorf("expected num edges %d, got %d\n", len(tc.expected), len(actual))
			}
			for _, expectedEdge := range tc.expected {
				found := false
				for _, actualEdge := range actual {
					if expectedEdge == actualEdge {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected edge (%s) not found\n", expectedEdge)
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

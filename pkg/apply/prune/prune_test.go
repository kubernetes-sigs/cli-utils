// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/taskrunner"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var testNamespace = "test-inventory-namespace"
var inventoryObjName = "test-inventory-obj"
var podName = "pod-1"
var pdbName = "pdb"
var roleName = "role"

var testInventoryLabel = "test-app-label"

var inventoryObj = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      inventoryObjName,
			"namespace": testNamespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: testInventoryLabel,
			},
		},
	},
}

var namespace = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": testNamespace,
			"uid":  "uid-namespace",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var pod = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      podName,
			"namespace": testNamespace,
			"uid":       "uid1",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var pdb = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "policy/v1beta1",
		"kind":       "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name":      pdbName,
			"namespace": testNamespace,
			"uid":       "uid2",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var pdbGetFailure = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "policy/v1beta1",
		"kind":       "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name":      pdbName + "get-failure",
			"namespace": testNamespace,
			"uid":       "uid2",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var pdbDeleteFailure = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "policy/v1beta1",
		"kind":       "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name":      pdbName + "delete-failure",
			"namespace": testNamespace,
			"uid":       "uid2",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var role = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata": map[string]interface{}{
			"name":      roleName,
			"namespace": testNamespace,
			"uid":       "uid3",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

var unknownCR = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "cli-utils.test/v1",
		"kind":       "Unknown",
		"metadata": map[string]interface{}{
			"name":      "test",
			"namespace": "default",
			"annotations": map[string]interface{}{
				"config.k8s.io/owning-inventory": testInventoryLabel,
			},
		},
	},
}

// Returns a inventory object with the inventory set from
// the passed "children".
func createInventoryInfo(children ...*unstructured.Unstructured) inventory.InventoryInfo {
	inventoryObjCopy := inventoryObj.DeepCopy()
	wrappedInv := inventory.WrapInventoryObj(inventoryObjCopy)
	objs := object.UnstructuredsToObjMetas(children)
	if err := wrappedInv.Store(objs); err != nil {
		return nil
	}
	obj, err := wrappedInv.GetObject()
	if err != nil {
		return nil
	}
	return inventory.WrapInventoryInfoObj(obj)
}

// preventDelete object contains the "on-remove:keep" lifecycle directive.
var preventDelete = &unstructured.Unstructured{
	Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "test-prevent-delete",
			"namespace": testNamespace,
			"annotations": map[string]interface{}{
				common.OnRemoveAnnotation: common.OnRemoveKeep,
			},
			"uid": "prevent-delete",
		},
	},
}

func TestPrune(t *testing.T) {
	tests := map[string]struct {
		// pastObjs/currentObjs do NOT contain the inventory object.
		// Inventory object is generated from these past/current objects.
		pastObjs    []*unstructured.Unstructured
		currentObjs []*unstructured.Unstructured
		prunedObjs  []*unstructured.Unstructured
		// finalClusterObjs are the objects in cluster at the end of prune,
		// and the objects which should be stored in the inventory object.
		finalClusterObjs []*unstructured.Unstructured
		pruneEventObjs   []*unstructured.Unstructured
	}{
		"Past and current objects are empty; no pruned objects": {
			pastObjs:         []*unstructured.Unstructured{},
			currentObjs:      []*unstructured.Unstructured{},
			prunedObjs:       []*unstructured.Unstructured{},
			finalClusterObjs: []*unstructured.Unstructured{},
			pruneEventObjs:   []*unstructured.Unstructured{},
		},
		"Past and current objects are the same; no pruned objects": {
			pastObjs:         []*unstructured.Unstructured{namespace, pdb},
			currentObjs:      []*unstructured.Unstructured{pdb, namespace},
			prunedObjs:       []*unstructured.Unstructured{},
			finalClusterObjs: []*unstructured.Unstructured{namespace, pdb},
			pruneEventObjs:   []*unstructured.Unstructured{},
		},
		"No past objects; no pruned objects": {
			pastObjs:         []*unstructured.Unstructured{},
			currentObjs:      []*unstructured.Unstructured{pdb, namespace},
			pruneEventObjs:   []*unstructured.Unstructured{},
			finalClusterObjs: []*unstructured.Unstructured{pdb, namespace},
			prunedObjs:       []*unstructured.Unstructured{},
		},
		"No current objects; all previous objects pruned in correct order": {
			pastObjs:         []*unstructured.Unstructured{pdb, role, pod},
			currentObjs:      []*unstructured.Unstructured{},
			prunedObjs:       []*unstructured.Unstructured{pod, pdb, role},
			finalClusterObjs: []*unstructured.Unstructured{},
			pruneEventObjs:   []*unstructured.Unstructured{pod, pdb, role},
		},
		"Omitted object is pruned": {
			pastObjs:         []*unstructured.Unstructured{pdb, role},
			currentObjs:      []*unstructured.Unstructured{pdb},
			prunedObjs:       []*unstructured.Unstructured{role},
			finalClusterObjs: []*unstructured.Unstructured{pdb},
			pruneEventObjs:   []*unstructured.Unstructured{role},
		},
		"Prevent delete lifecycle annotation stops pruning": {
			pastObjs:         []*unstructured.Unstructured{preventDelete, pdb},
			currentObjs:      []*unstructured.Unstructured{pdb, role},
			prunedObjs:       []*unstructured.Unstructured{},
			finalClusterObjs: []*unstructured.Unstructured{preventDelete, pdb, role},
			pruneEventObjs:   []*unstructured.Unstructured{preventDelete},
		},
		"Namespace not pruned if objects are still in it": {
			pastObjs:         []*unstructured.Unstructured{namespace, pdb, pod},
			currentObjs:      []*unstructured.Unstructured{pod},
			prunedObjs:       []*unstructured.Unstructured{pdb},
			finalClusterObjs: []*unstructured.Unstructured{namespace, pod},
			pruneEventObjs:   []*unstructured.Unstructured{pdb, namespace},
		},
		"unknown type doesn't emit prune failed event": {
			pastObjs:         []*unstructured.Unstructured{unknownCR},
			currentObjs:      []*unstructured.Unstructured{},
			prunedObjs:       []*unstructured.Unstructured{unknownCR},
			finalClusterObjs: []*unstructured.Unstructured{},
			pruneEventObjs:   []*unstructured.Unstructured{},
		},
	}
	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				po := NewPruneOptions()
				// Set up the union of previously applied objects and the
				// currently applied objects as the current inventory items.
				clusterObjs := object.UnstructuredsToObjMetas(tc.pastObjs)
				currentObjs := object.UnstructuredsToObjMetas(tc.currentObjs)
				fakeInvClient := inventory.NewFakeInventoryClient(object.Union(clusterObjs, currentObjs))
				po.InvClient = fakeInvClient
				// Set up the current inventory with union of objects.
				unionObjs := unionObjects(tc.pastObjs, tc.currentObjs)
				currentInventory := createInventoryInfo(unionObjs...)
				// Set up the fake dynamic client to recognize all objects, and the RESTMapper.
				objs := []runtime.Object{}
				for _, obj := range unionObjs {
					objs = append(objs, obj)
				}
				po.client = fake.NewSimpleDynamicClient(scheme.Scheme, objs...)
				po.mapper = testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...)
				// The event channel can not block; make sure its bigger than all
				// the events that can be put on it.
				eventChannel := make(chan event.Event, len(tc.pastObjs)+1) // Add one for inventory object
				taskContext := taskrunner.NewTaskContext(eventChannel)
				for _, u := range tc.currentObjs {
					o := object.UnstructuredToObjMeta(u)
					uid := u.GetUID()
					taskContext.ResourceApplied(o, uid, 0)
				}
				err := func() error {
					defer close(eventChannel)
					// Run the prune and validate.
					return po.Prune(currentInventory, tc.currentObjs, populateObjectIds(tc.currentObjs, t), taskContext, Options{
						DryRunStrategy: drs,
					})
				}()

				if err != nil {
					t.Fatalf("Unexpected error during Prune(): %#v", err)
				}

				// Test that the correct inventory objects are stored at the end of the prune.
				actualObjs := fakeInvClient.Objs
				expectedObjs := object.UnstructuredsToObjMetas(tc.finalClusterObjs)
				if !object.SetEquals(expectedObjs, actualObjs) {
					t.Errorf("expected inventory objs (%s), got (%s)", expectedObjs, actualObjs)
				}

				var actualPruneEvents []event.Event
				for e := range eventChannel {
					actualPruneEvents = append(actualPruneEvents, e)
				}
				if want, got := len(tc.pruneEventObjs), len(actualPruneEvents); want != got {
					t.Errorf("Expected (%d) prune events, got (%d)", want, got)
				}

				for i, obj := range tc.pruneEventObjs {
					e := actualPruneEvents[i]
					expKind := obj.GetObjectKind().GroupVersionKind().Kind
					actKind := e.PruneEvent.Identifier.GroupKind.Kind
					if expKind != actKind {
						t.Errorf("Expected kind %s, got %s", expKind, actKind)
					}
				}
			})
		}
	}
}

// unionObjects returns the union of sliceA and sliceB as a slice of unstructured objects.
func unionObjects(sliceA []*unstructured.Unstructured, sliceB []*unstructured.Unstructured) []*unstructured.Unstructured {
	m := map[string]*unstructured.Unstructured{}
	for _, a := range sliceA {
		metadata := object.UnstructuredToObjMeta(a)
		m[metadata.String()] = a
	}
	for _, b := range sliceB {
		metadata := object.UnstructuredToObjMeta(b)
		m[metadata.String()] = b
	}
	union := []*unstructured.Unstructured{}
	for _, u := range m {
		union = append(union, u)
	}
	return union
}

// populateObjectIds returns a pointer to a set of strings containing
// the UID's of the passed objects (infos).
func populateObjectIds(objs []*unstructured.Unstructured, t *testing.T) sets.String {
	uids := sets.NewString()
	for _, currObj := range objs {
		metadata, err := meta.Accessor(currObj)
		if err != nil {
			t.Fatalf("Unexpected error retrieving object metadata: %#v", err)
		}
		uid := string(metadata.GetUID())
		uids.Insert(uid)
	}
	return uids
}

func TestPreventDeleteAnnotation(t *testing.T) {
	tests := map[string]struct {
		annotations map[string]string
		expected    bool
	}{
		"Nil map returns false": {
			annotations: nil,
			expected:    false,
		},
		"Empty map returns false": {
			annotations: map[string]string{},
			expected:    false,
		},
		"Wrong annotation key/value is false": {
			annotations: map[string]string{
				"foo": "bar",
			},
			expected: false,
		},
		"Annotation key without value is false": {
			annotations: map[string]string{
				common.OnRemoveAnnotation: "bar",
			},
			expected: false,
		},
		"Annotation key and value is true": {
			annotations: map[string]string{
				common.OnRemoveAnnotation: common.OnRemoveKeep,
			},
			expected: true,
		},
		"Annotation key client.lifecycle.config.k8s.io/deletion without value is false": {
			annotations: map[string]string{
				common.LifecycleDeleteAnnotation: "any",
			},
			expected: false,
		},
		"Annotation key client.lifecycle.config.k8s.io/deletion and value is true": {
			annotations: map[string]string{
				common.LifecycleDeleteAnnotation: common.PreventDeletion,
			},
			expected: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := preventDeleteAnnotation(tc.annotations)
			if tc.expected != actual {
				t.Errorf("preventDeleteAnnotation Expected (%t), got (%t)", tc.expected, actual)
			}
		})
	}
}

func TestPruneWithError(t *testing.T) {
	tests := map[string]struct {
		// pastObjs/currentObjs do NOT contain the inventory object.
		// Inventory object is generated from these past/current objects.
		pastObjs    []*unstructured.Unstructured
		currentObjs []*unstructured.Unstructured
		prunedEvent []event.Event
		isError     bool
	}{
		"some objects have failure to get": {
			pastObjs:    []*unstructured.Unstructured{pdbGetFailure, role},
			currentObjs: []*unstructured.Unstructured{},
			prunedEvent: []event.Event{
				{
					Type: event.PruneType,
					PruneEvent: event.PruneEvent{
						Identifier: object.ObjMetadata{
							Name:      pdbName + "get-failure",
							Namespace: testNamespace,
							GroupKind: schema.GroupKind{
								Group: "policy/v1beta1",
								Kind:  "PodDisruptionBudget",
							},
						},
						Error: fmt.Errorf("expected get error"),
					},
				},
				{
					Type: event.PruneType,
					PruneEvent: event.PruneEvent{
						Identifier: object.ObjMetadata{
							Name:      roleName,
							Namespace: testNamespace,
							GroupKind: schema.GroupKind{
								Group: "v1",
								Kind:  "Role",
							},
						},
						Error: nil,
					},
				},
			},
			isError: false,
		},
		"some objects have failure to delete": {
			pastObjs:    []*unstructured.Unstructured{pdbDeleteFailure, role},
			currentObjs: []*unstructured.Unstructured{},
			prunedEvent: []event.Event{
				{
					Type: event.PruneType,
					PruneEvent: event.PruneEvent{
						Identifier: object.ObjMetadata{
							Name:      pdbName + "delete-failure",
							Namespace: testNamespace,
							GroupKind: schema.GroupKind{
								Group: "policy/v1beta1",
								Kind:  "PodDisruptionBudget",
							},
						},
						Error: fmt.Errorf("expected delete error"),
					},
				},
				{
					Type: event.PruneType,
					PruneEvent: event.PruneEvent{
						Identifier: object.ObjMetadata{
							Name:      roleName,
							Namespace: testNamespace,
							GroupKind: schema.GroupKind{
								Group: "v1",
								Kind:  "Role",
							},
						},
						Error: nil,
					},
				},
			},
			isError: false,
		},
	}
	for name, tc := range tests {
		drs := common.DryRunNone
		t.Run(name, func(t *testing.T) {
			po := NewPruneOptions()
			// Set up the previously applied objects.
			clusterObjs := object.UnstructuredsToObjMetas(tc.pastObjs)
			po.InvClient = inventory.NewFakeInventoryClient(clusterObjs)
			// Set up the currently applied objects.
			currentInventory := createInventoryInfo(tc.currentObjs...)
			// Set up the fake dynamic client to recognize all objects, and the RESTMapper.
			po.client = &fakeDynamicFailureClient{dynamic: fake.NewSimpleDynamicClient(scheme.Scheme,
				namespace, pdb, role)}
			// po.client = fake.NewSimpleDynamicClient(scheme.Scheme, namespace, pdb, role)
			po.mapper = testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
				scheme.Scheme.PrioritizedVersionsAllGroups()...)
			// The event channel can not block; make sure its bigger than all
			// the events that can be put on it.
			eventChannel := make(chan event.Event, len(tc.pastObjs)+1) // Add one for inventory object
			taskContext := taskrunner.NewTaskContext(eventChannel)
			err := func() error {
				defer close(eventChannel)
				// Run the prune and validate.
				return po.Prune(currentInventory, tc.currentObjs, populateObjectIds(tc.currentObjs, t), taskContext, Options{
					DryRunStrategy: drs,
				})
			}()
			if !tc.isError {
				if err != nil {
					t.Fatalf("Unexpected error during Prune(): %#v", err)
				}

				var actualPruneEvents []event.Event
				for e := range eventChannel {
					actualPruneEvents = append(actualPruneEvents, e)
				}
				if want, got := len(tc.prunedEvent), len(actualPruneEvents); want != got {
					t.Errorf("Expected (%d) prune events, got (%d)", want, got)
				}

				for i, expectedEvt := range tc.prunedEvent {
					e := actualPruneEvents[i]
					expKind := expectedEvt.PruneEvent.Identifier.GroupKind.Kind
					actKind := e.PruneEvent.Identifier.GroupKind.Kind
					if expKind != actKind {
						t.Errorf("Expected kind %s, got %s", expKind, actKind)
					}
					if !reflect.DeepEqual(e.PruneEvent.Error, expectedEvt.PruneEvent.Error) {
						t.Errorf("Expected error %q, got %q", expectedEvt.PruneEvent.Error, e.PruneEvent.Error)
					}
				}
			} else if err == nil {
				t.Fatalf("Expected error during Prune() but received none")
			}
		})
	}
}

type fakeDynamicFailureClient struct {
	dynamic dynamic.Interface
}

var _ dynamic.Interface = &fakeDynamicFailureClient{}

func (c *fakeDynamicFailureClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	if resource.Resource == "poddisruptionbudgets" {
		return &fakeDynamicResourceClient{NamespaceableResourceInterface: c.dynamic.Resource(resource)}
	}
	return c.dynamic.Resource(resource)
}

type fakeDynamicResourceClient struct {
	dynamic.NamespaceableResourceInterface
}

func (c *fakeDynamicResourceClient) Namespace(ns string) dynamic.ResourceInterface {
	return &fakeNamespaceClient{ResourceInterface: c.NamespaceableResourceInterface.Namespace(ns)}
}

// fakeNamespaceClient wrappers around a namespaceClient with the overwriting to Get and Delete functions.
type fakeNamespaceClient struct {
	dynamic.ResourceInterface
}

var _ dynamic.ResourceInterface = &fakeNamespaceClient{}

func (c *fakeNamespaceClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
	if strings.Contains(name, "delete-failure") {
		return fmt.Errorf("expected delete error")
	}
	return nil
}

func (c *fakeNamespaceClient) Get(ctx context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	if strings.Contains(name, "get-failure") {
		return nil, fmt.Errorf("expected get error")
	}
	return pdb, nil
}

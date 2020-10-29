// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package prune

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var testNamespace = "test-inventory-namespace"
var inventoryObjName = "test-inventory-obj"
var namespaceName = "namespace"
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
			"name":      namespaceName,
			"namespace": testNamespace,
			"uid":       "uid1",
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
		},
	},
}

// Returns a inventory object with the inventory set from
// the passed "children".
func createInventoryInfo(children ...*unstructured.Unstructured) *unstructured.Unstructured {
	inventoryObjCopy := inventoryObj.DeepCopy()
	wrappedInv := inventory.WrapInventoryObj(inventoryObjCopy)
	objs := object.UnstructuredsToObjMetas(children)
	if err := wrappedInv.Store(objs); err != nil {
		return nil
	}
	inventoryInfo, _ := wrappedInv.GetObject()
	return inventoryInfo
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
		isError     bool
	}{
		"Past and current objects are empty; no pruned objects": {
			pastObjs:    []*unstructured.Unstructured{},
			currentObjs: []*unstructured.Unstructured{},
			prunedObjs:  []*unstructured.Unstructured{},
			isError:     false,
		},
		"Past and current objects are the same; no pruned objects": {
			pastObjs:    []*unstructured.Unstructured{namespace, pdb},
			currentObjs: []*unstructured.Unstructured{pdb, namespace},
			prunedObjs:  []*unstructured.Unstructured{},
			isError:     false,
		},
		"No past objects; no pruned objects": {
			pastObjs:    []*unstructured.Unstructured{},
			currentObjs: []*unstructured.Unstructured{pdb, namespace},
			prunedObjs:  []*unstructured.Unstructured{},
			isError:     false,
		},
		"No current objects; all previous objects pruned in correct order": {
			pastObjs:    []*unstructured.Unstructured{namespace, pdb, role},
			currentObjs: []*unstructured.Unstructured{},
			prunedObjs:  []*unstructured.Unstructured{pdb, role, namespace},
			isError:     false,
		},
		"Omitted object is pruned": {
			pastObjs:    []*unstructured.Unstructured{namespace, pdb},
			currentObjs: []*unstructured.Unstructured{pdb, role},
			prunedObjs:  []*unstructured.Unstructured{namespace},
			isError:     false,
		},
		"Prevent delete lifecycle annotation stops pruning": {
			pastObjs:    []*unstructured.Unstructured{preventDelete, pdb},
			currentObjs: []*unstructured.Unstructured{pdb, role},
			prunedObjs:  []*unstructured.Unstructured{},
			isError:     false,
		},
	}
	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				po := NewPruneOptions()
				// Set up the previously applied objects.
				clusterObjs := object.UnstructuredsToObjMetas(tc.pastObjs)
				po.InvClient = inventory.NewFakeInventoryClient(clusterObjs)
				// Set up the currently applied objects.
				currentInventory := createInventoryInfo(tc.currentObjs...)
				// Set up the fake dynamic client to recognize all objects, and the RESTMapper.
				po.client = fake.NewSimpleDynamicClient(scheme.Scheme,
					namespace, pdb, role)
				po.mapper = testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
					scheme.Scheme.PrioritizedVersionsAllGroups()...)
				// The event channel can not block; make sure its bigger than all
				// the events that can be put on it.
				eventChannel := make(chan event.Event, len(tc.pastObjs)+1) // Add one for inventory object
				err := func() error {
					defer close(eventChannel)
					// Run the prune and validate.
					return po.Prune(currentInventory, tc.currentObjs, populateObjectIds(tc.currentObjs, t), eventChannel, Options{
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
					if want, got := len(tc.prunedObjs), len(actualPruneEvents); want != got {
						t.Errorf("Expected (%d) prune events, got (%d)", want, got)
					}

					for i, obj := range tc.prunedObjs {
						e := actualPruneEvents[i]
						expKind := obj.GetObjectKind().GroupVersionKind().Kind
						actKind := e.PruneEvent.Object.GetObjectKind().GroupVersionKind().Kind
						if expKind != actKind {
							t.Errorf("Expected kind %s, got %s", expKind, actKind)
						}
					}
				} else if err == nil {
					t.Fatalf("Expected error during Prune() but received none")
				}
			})
		}
	}
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

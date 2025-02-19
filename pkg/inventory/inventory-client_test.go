// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	clienttesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func podStatus(info *resource.Info) actuation.ObjectStatus {
	return actuation.ObjectStatus{
		ObjectReference: ObjectReferenceFromObjMetadata(ignoreErrInfoToObjMeta(info)),
		Strategy:        actuation.ActuationStrategyApply,
		Actuation:       actuation.ActuationSucceeded,
		Reconcile:       actuation.ReconcileSucceeded,
	}
}

func podData(name string) map[string]string {
	return map[string]string{
		fmt.Sprintf("test-inventory-namespace_%s__Pod", name): "{\"actuation\":\"Succeeded\",\"reconcile\":\"Succeeded\",\"strategy\":\"Apply\"}",
	}
}

func podDataNoStatus(name string) map[string]string {
	return map[string]string{
		fmt.Sprintf("test-inventory-namespace_%s__Pod", name): "",
	}
}

func TestGetClusterInventoryInfo(t *testing.T) {
	tests := map[string]struct {
		statusPolicy StatusPolicy
		inv          Info
		localObjs    object.ObjMetadataSet
		objStatus    []actuation.ObjectStatus
		isError      bool
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: object.ObjMetadataSet{},
			isError:   true,
		},
		"Empty local inventory object": {
			inv:       localInv,
			localObjs: object.ObjMetadataSet{},
			isError:   false,
		},
		"Local inventory with a single object": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			objStatus: []actuation.ObjectStatus{podStatus(pod2Info)},
			isError:   false,
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			objStatus: []actuation.ObjectStatus{
				podStatus(pod1Info),
				podStatus(pod2Info),
				podStatus(pod3Info),
			},
			isError: false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, err := NewClient(tf,
				WrapInventoryObj, InvInfoToConfigMap, tc.statusPolicy, ConfigMapGVK)
			require.NoError(t, err)

			var inv *unstructured.Unstructured
			if tc.inv != nil {
				inv = storeObjsInInventory(tc.inv, tc.localObjs, tc.objStatus)
			}
			clusterInv, err := invClient.getClusterInventoryInfo(t.Context(), WrapInventoryInfoObj(inv))
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error but received none")
				}
				return
			}
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if clusterInv != nil {
				wrapped := WrapInventoryObj(clusterInv)
				clusterObjs, err := wrapped.Load()
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
				if !tc.localObjs.Equal(clusterObjs) {
					t.Fatalf("expected cluster objs (%v), got (%v)", tc.localObjs, clusterObjs)
				}
			}
		})
	}
}

func TestMerge(t *testing.T) {
	tests := map[string]struct {
		statusPolicy StatusPolicy
		localInv     Info
		localObjs    object.ObjMetadataSet
		clusterObjs  object.ObjMetadataSet
		pruneObjs    object.ObjMetadataSet
		isError      bool
	}{
		"Nil local inventory object is error": {
			localInv:     nil,
			localObjs:    object.ObjMetadataSet{},
			clusterObjs:  object.ObjMetadataSet{},
			pruneObjs:    object.ObjMetadataSet{},
			isError:      true,
			statusPolicy: StatusPolicyAll,
		},
		"Cluster and local inventories empty: no prune objects; no change": {
			localInv:     copyInventory(),
			localObjs:    object.ObjMetadataSet{},
			clusterObjs:  object.ObjMetadataSet{},
			pruneObjs:    object.ObjMetadataSet{},
			isError:      false,
			statusPolicy: StatusPolicyAll,
		},
		"Cluster and local inventories same: no prune objects; no change": {
			localInv: copyInventory(),
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			pruneObjs:    object.ObjMetadataSet{},
			isError:      false,
			statusPolicy: StatusPolicyAll,
		},
		"Cluster two obj, local one: prune obj": {
			localInv: copyInventory(),
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			pruneObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod3Info),
			},
			statusPolicy: StatusPolicyAll,
			isError:      false,
		},
		"Cluster multiple objs, local multiple different objs: prune objs": {
			localInv: copyInventory(),
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			pruneObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			statusPolicy: StatusPolicyAll,
			isError:      false,
		},
	}

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
				defer tf.Cleanup()

				tf.FakeDynamicClient.PrependReactor("list", "configmaps", toReactionFunc(tc.clusterObjs))
				// Create the local inventory object storing "tc.localObjs"
				invClient, err := NewClient(tf,
					WrapInventoryObj, InvInfoToConfigMap, tc.statusPolicy, ConfigMapGVK)
				require.NoError(t, err)

				// Call "Merge" to create the union of clusterObjs and localObjs.
				pruneObjs, err := invClient.Merge(t.Context(), tc.localInv, tc.localObjs, drs)
				if tc.isError {
					if err == nil {
						t.Fatalf("expected error but received none")
					}
					return
				}
				if !tc.isError && err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if !tc.pruneObjs.Equal(pruneObjs) {
					t.Errorf("expected (%v) prune objs; got (%v)", tc.pruneObjs, pruneObjs)
				}
			})
		}
	}
}

func TestReplace(t *testing.T) {
	tests := map[string]struct {
		statusPolicy StatusPolicy
		localObjs    object.ObjMetadataSet
		clusterObjs  object.ObjMetadataSet
		objStatus    []actuation.ObjectStatus
		data         map[string]string
	}{
		"Cluster and local inventories empty": {
			localObjs:   object.ObjMetadataSet{},
			clusterObjs: object.ObjMetadataSet{},
			data:        map[string]string{},
		},
		"Cluster and local inventories same": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			objStatus:    []actuation.ObjectStatus{podStatus(pod1Info)},
			data:         podData("pod-1"),
			statusPolicy: StatusPolicyAll,
		},
		"Cluster two obj, local one": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			objStatus:    []actuation.ObjectStatus{podStatus(pod1Info), podStatus(pod3Info)},
			data:         podData("pod-1"),
			statusPolicy: StatusPolicyAll,
		},
		"Cluster multiple objs, local multiple different objs": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			objStatus:    []actuation.ObjectStatus{podStatus(pod2Info), podStatus(pod1Info), podStatus(pod3Info)},
			data:         podData("pod-2"),
			statusPolicy: StatusPolicyAll,
		},
		"Cluster multiple objs, local multiple different objs with StatusPolicyNone": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			objStatus:    []actuation.ObjectStatus{podStatus(pod2Info), podStatus(pod1Info), podStatus(pod3Info)},
			data:         podDataNoStatus("pod-2"),
			statusPolicy: StatusPolicyNone,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	// Client and server dry-run do not throw errors.
	invClient, err := NewClient(tf,
		WrapInventoryObj, InvInfoToConfigMap, StatusPolicyAll, ConfigMapGVK)
	require.NoError(t, err)
	err = invClient.Replace(t.Context(), copyInventory(), object.ObjMetadataSet{}, nil, common.DryRunClient)
	if err != nil {
		t.Fatalf("unexpected error received: %s", err)
	}
	err = invClient.Replace(t.Context(), copyInventory(), object.ObjMetadataSet{}, nil, common.DryRunServer)
	if err != nil {
		t.Fatalf("unexpected error received: %s", err)
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create inventory client, and store the cluster objs in the inventory object.
			invClient, err := NewClient(tf,
				WrapInventoryObj, InvInfoToConfigMap, tc.statusPolicy, ConfigMapGVK)
			require.NoError(t, err)
			wrappedInv := invClient.InventoryFactoryFunc(inventoryObj)
			if err := wrappedInv.Store(tc.clusterObjs, tc.objStatus); err != nil {
				t.Fatalf("unexpected error storing inventory objects: %s", err)
			}
			inv, err := wrappedInv.GetObject()
			if err != nil {
				t.Fatalf("unexpected error storing inventory objects: %s", err)
			}
			// Call replaceInventory with the new set of "localObjs"
			inv, _, err = invClient.replaceInventory(inv, tc.localObjs, tc.objStatus)
			if err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			wrappedInv = invClient.InventoryFactoryFunc(inv)
			// Validate that the stored objects are now the "localObjs".
			actualObjs, err := wrappedInv.Load()
			if err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if !tc.localObjs.Equal(actualObjs) {
				t.Errorf("expected objects (%s), got (%s)", tc.localObjs, actualObjs)
			}
			data, _, err := unstructured.NestedStringMap(inv.Object, "data")
			if err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if diff := cmp.Diff(data, tc.data); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestGetClusterObjs(t *testing.T) {
	tests := map[string]struct {
		statusPolicy StatusPolicy
		localInv     Info
		clusterObjs  object.ObjMetadataSet
		isError      bool
	}{
		"Nil cluster inventory is error": {
			localInv:    nil,
			clusterObjs: object.ObjMetadataSet{},
			isError:     true,
		},
		"No cluster objs": {
			localInv:    copyInventory(),
			clusterObjs: object.ObjMetadataSet{},
			isError:     false,
		},
		"Single cluster obj": {
			localInv:    copyInventory(),
			clusterObjs: object.ObjMetadataSet{ignoreErrInfoToObjMeta(pod1Info)},
			isError:     false,
		},
		"Multiple cluster objs": {
			localInv:    copyInventory(),
			clusterObjs: object.ObjMetadataSet{ignoreErrInfoToObjMeta(pod1Info), ignoreErrInfoToObjMeta(pod3Info)},
			isError:     false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()
			tf.FakeDynamicClient.PrependReactor("list", "configmaps", toReactionFunc(tc.clusterObjs))

			invClient, err := NewClient(tf,
				WrapInventoryObj, InvInfoToConfigMap, tc.statusPolicy, ConfigMapGVK)
			require.NoError(t, err)
			clusterObjs, err := invClient.GetClusterObjs(t.Context(), tc.localInv)
			if tc.isError {
				if err == nil {
					t.Fatalf("expected error but received none")
				}
				return
			}
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if !tc.clusterObjs.Equal(clusterObjs) {
				t.Errorf("expected (%v) cluster inventory objs; got (%v)", tc.clusterObjs, clusterObjs)
			}
		})
	}
}

func TestDeleteInventoryObj(t *testing.T) {
	tests := map[string]struct {
		statusPolicy StatusPolicy
		inv          Info
		localObjs    object.ObjMetadataSet
		objStatus    []actuation.ObjectStatus
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: object.ObjMetadataSet{},
		},
		"Empty local inventory object": {
			inv:       localInv,
			localObjs: object.ObjMetadataSet{},
		},
		"Local inventory with a single object": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			objStatus: []actuation.ObjectStatus{podStatus(pod2Info)},
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			objStatus: []actuation.ObjectStatus{
				podStatus(pod1Info),
				podStatus(pod2Info),
				podStatus(pod3Info),
			},
		},
	}

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
				defer tf.Cleanup()

				invClient, err := NewClient(tf,
					WrapInventoryObj, InvInfoToConfigMap, tc.statusPolicy, ConfigMapGVK)
				require.NoError(t, err)
				inv := invClient.invToUnstructuredFunc(tc.inv)
				if inv != nil {
					inv = storeObjsInInventory(tc.inv, tc.localObjs, tc.objStatus)
				}
				err = invClient.deleteInventoryObjByName(t.Context(), inv, drs)
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
			})
		}
	}
}

func ignoreErrInfoToObjMeta(info *resource.Info) object.ObjMetadata {
	objMeta, _ := object.InfoToObjMeta(info)
	return objMeta
}

func toReactionFunc(objs object.ObjMetadataSet) clienttesting.ReactionFunc {
	return func(action clienttesting.Action) (bool, runtime.Object, error) {
		u := copyInventoryInfo()
		err := unstructured.SetNestedStringMap(u.Object, objs.ToStringMap(), "data")
		if err != nil {
			return true, nil, err
		}
		list := &unstructured.UnstructuredList{}
		list.Items = []unstructured.Unstructured{*u}
		return true, list, err
	}
}

func storeObjsInInventory(info Info, objs object.ObjMetadataSet, status []actuation.ObjectStatus) *unstructured.Unstructured {
	wrapped := WrapInventoryObj(InvInfoToConfigMap(info))
	_ = wrapped.Store(objs, status)
	inv, _ := wrapped.GetObject()
	return inv
}

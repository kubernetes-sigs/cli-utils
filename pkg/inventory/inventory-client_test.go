// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	clienttesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestGetClusterInventoryInfo(t *testing.T) {
	tests := map[string]struct {
		inv       InventoryInfo
		localObjs object.ObjMetadataSet
		isError   bool
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
			isError: false,
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			isError: false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)

			var inv *unstructured.Unstructured
			if tc.inv != nil {
				inv = storeObjsInInventory(tc.inv, tc.localObjs)
			}
			clusterInv, err := invClient.GetClusterInventoryInfo(WrapInventoryInfoObj(inv))
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
		localInv    InventoryInfo
		localObjs   object.ObjMetadataSet
		clusterObjs object.ObjMetadataSet
		pruneObjs   object.ObjMetadataSet
		isError     bool
	}{
		"Nil local inventory object is error": {
			localInv:    nil,
			localObjs:   object.ObjMetadataSet{},
			clusterObjs: object.ObjMetadataSet{},
			pruneObjs:   object.ObjMetadataSet{},
			isError:     true,
		},
		"Cluster and local inventories empty: no prune objects; no change": {
			localInv:    copyInventory(),
			localObjs:   object.ObjMetadataSet{},
			clusterObjs: object.ObjMetadataSet{},
			pruneObjs:   object.ObjMetadataSet{},
			isError:     false,
		},
		"Cluster and local inventories same: no prune objects; no change": {
			localInv: copyInventory(),
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			pruneObjs: object.ObjMetadataSet{},
			isError:   false,
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
			isError: false,
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
			isError: false,
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
				invClient, err := NewInventoryClient(tf,
					WrapInventoryObj, InvInfoToConfigMap)
				require.NoError(t, err)

				// Call "Merge" to create the union of clusterObjs and localObjs.
				pruneObjs, err := invClient.Merge(tc.localInv, tc.localObjs, drs)
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

func TestCreateInventory(t *testing.T) {
	tests := map[string]struct {
		inv       InventoryInfo
		localObjs object.ObjMetadataSet
		error     string
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: object.ObjMetadataSet{},
			error:     "attempting create a nil inventory object",
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
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()

			var storedInventory map[string]string
			tf.FakeDynamicClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				obj := *action.(clienttesting.CreateAction).GetObject().(*unstructured.Unstructured)
				storedInventory, _, _ = unstructured.NestedStringMap(obj.Object, "data")
				return true, nil, nil
			})

			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)
			inv := invClient.invToUnstructuredFunc(tc.inv)
			if inv != nil {
				inv = storeObjsInInventory(tc.inv, tc.localObjs)
			}
			err = invClient.createInventoryObj(inv, common.DryRunNone)
			if tc.error != "" {
				assert.EqualError(t, err, tc.error)
			} else {
				assert.NoError(t, err)
			}

			expectedInventory := tc.localObjs.ToStringMap()
			// handle empty inventories special to avoid problems with empty vs nil maps
			if len(expectedInventory) != 0 || len(storedInventory) != 0 {
				assert.Equal(t, expectedInventory, storedInventory)
			}
		})
	}
}

func TestReplace(t *testing.T) {
	tests := map[string]struct {
		localObjs   object.ObjMetadataSet
		clusterObjs object.ObjMetadataSet
	}{
		"Cluster and local inventories empty": {
			localObjs:   object.ObjMetadataSet{},
			clusterObjs: object.ObjMetadataSet{},
		},
		"Cluster and local inventories same": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
		},
		"Cluster two obj, local one": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
		},
		"Cluster multiple objs, local multiple different objs": {
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	// Client and server dry-run do not throw errors.
	invClient, err := NewInventoryClient(tf, WrapInventoryObj, InvInfoToConfigMap)
	require.NoError(t, err)
	err = invClient.Replace(copyInventory(), object.ObjMetadataSet{}, common.DryRunClient, nil)
	if err != nil {
		t.Fatalf("unexpected error received: %s", err)
	}
	err = invClient.Replace(copyInventory(), object.ObjMetadataSet{}, common.DryRunServer, nil)
	if err != nil {
		t.Fatalf("unexpected error received: %s", err)
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create inventory client, and store the cluster objs in the inventory object.
			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)
			wrappedInv := invClient.InventoryFactoryFunc(inventoryObj)
			if err := wrappedInv.Store(tc.clusterObjs, nil); err != nil {
				t.Fatalf("unexpected error storing inventory objects: %s", err)
			}
			inv, err := wrappedInv.GetObject()
			if err != nil {
				t.Fatalf("unexpected error storing inventory objects: %s", err)
			}
			// Call replaceInventory with the new set of "localObjs"
			inv, err = invClient.replaceInventory(inv, tc.localObjs, nil)
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
		})
	}
}

func TestGetClusterObjs(t *testing.T) {
	tests := map[string]struct {
		localInv    InventoryInfo
		clusterObjs object.ObjMetadataSet
		isError     bool
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

			invClient, err := NewInventoryClient(tf,
				WrapInventoryObj, InvInfoToConfigMap)
			require.NoError(t, err)
			clusterObjs, err := invClient.GetClusterObjs(tc.localInv)
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
		inv       InventoryInfo
		localObjs object.ObjMetadataSet
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
		},
		"Local inventory with multiple objects": {
			inv: localInv,
			localObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
		},
	}

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
				defer tf.Cleanup()

				invClient, err := NewInventoryClient(tf,
					WrapInventoryObj, InvInfoToConfigMap)
				require.NoError(t, err)
				inv := invClient.invToUnstructuredFunc(tc.inv)
				if inv != nil {
					inv = storeObjsInInventory(tc.inv, tc.localObjs)
				}
				err = invClient.deleteInventoryObjByName(inv, drs)
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

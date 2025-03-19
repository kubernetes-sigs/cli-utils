// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	clienttesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
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

func TestGet(t *testing.T) {
	localInv, err := ConfigMapToInventoryInfo(inventoryObj)
	require.NoError(t, err)
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()
			tf.FakeDynamicClient.PrependReactor("get", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				cm, _ := inventoryToConfigMap(&UnstructuredInventory{
					ClusterObj: copyInventoryInfo(),
					BaseInventory: BaseInventory{
						Objs:        tc.localObjs,
						ObjStatuses: tc.objStatus,
					},
				})
				return true, cm, nil
			})
			invClient, err := NewUnstructuredClient(tf,
				configMapToInventory, inventoryToConfigMap, ConfigMapGVK)
			require.NoError(t, err)

			clusterInv, err := invClient.Get(t.Context(), tc.inv, GetOptions{})
			if tc.isError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if clusterInv != nil {
				if !tc.localObjs.Equal(clusterInv.ObjectRefs()) {
					t.Fatalf("expected cluster objs (%v), got (%v)", tc.localObjs, clusterInv.ObjectRefs())
				}
			}
		})
	}
}

func TestCreateOrUpdate(t *testing.T) {
	tests := map[string]struct {
		inventory  *UnstructuredInventory
		createObjs object.ObjMetadataSet
		updateObjs object.ObjMetadataSet
		isError    bool
	}{
		"Nil local inventory object is error": {
			inventory:  nil,
			createObjs: object.ObjMetadataSet{},
			isError:    true,
		},
		"Create and update inventory with empty object set": {
			inventory: &UnstructuredInventory{
				ClusterObj: copyInventoryInfo(),
			},
			createObjs: object.ObjMetadataSet{},
			updateObjs: object.ObjMetadataSet{},
			isError:    false,
		},
		"Create and Update inventory with identical object set": {
			inventory: &UnstructuredInventory{
				ClusterObj: copyInventoryInfo(),
			},
			createObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			updateObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			isError: false,
		},
		"Create and Update inventory with expanding object set": {
			inventory: &UnstructuredInventory{
				ClusterObj: copyInventoryInfo(),
			},
			createObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			updateObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			isError: false,
		},
		"Create and Update inventory with shrinking object set": {
			inventory: &UnstructuredInventory{
				ClusterObj: copyInventoryInfo(),
			},
			createObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			updateObjs: object.ObjMetadataSet{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			isError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()

			var updateCalls int
			var createCalls int
			tf.FakeDynamicClient.PrependReactor("update", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				updateCalls++
				return false, nil, nil
			})
			tf.FakeDynamicClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				createCalls++
				return false, nil, nil
			})

			// Create the local inventory object storing "tc.localObjs"
			invClient, err := NewUnstructuredClient(tf,
				configMapToInventory, inventoryToConfigMap, ConfigMapGVK)
			require.NoError(t, err)

			inventory := tc.inventory
			if inventory != nil {
				inventory.SetObjectRefs(tc.createObjs)
			}
			// Call Update an initial time should create the object
			err = invClient.CreateOrUpdate(context.TODO(), inventory, UpdateOptions{})
			if tc.isError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if updateCalls != 0 || createCalls != 1 { // Update should fail, causing create
				t.Fatalf("expected 0 update but got %d and 1 create but got %d", updateCalls, createCalls)
			}
			inv, err := invClient.Get(context.TODO(), tc.inventory, GetOptions{})
			require.NoError(t, err)
			if !tc.createObjs.Equal(inv.ObjectRefs()) {
				t.Fatalf("expected %v to equal %v", tc.createObjs, inv.ObjectRefs())
			}

			inventory.SetObjectRefs(tc.updateObjs)
			// Call Update a second time should update the existing object
			err = invClient.CreateOrUpdate(context.TODO(), inventory, UpdateOptions{})
			require.NoError(t, err)
			if updateCalls != 1 || createCalls != 1 { // Update should succeed, create not called again
				t.Fatalf("expected 1 update but got %d and 1 create but got %d", updateCalls, createCalls)
			}
			inv, err = invClient.Get(context.TODO(), tc.inventory, GetOptions{})
			require.NoError(t, err)
			if !tc.updateObjs.Equal(inv.ObjectRefs()) {
				t.Fatalf("expected %v to equal %v", tc.updateObjs, inv.ObjectRefs())
			}
		})
	}
}

func TestDelete(t *testing.T) {
	localInv, err := ConfigMapToInventoryInfo(inventoryObj)
	require.NoError(t, err)
	tests := map[string]struct {
		statusPolicy StatusPolicy
		inv          Info
		localObjs    object.ObjMetadataSet
		objStatus    []actuation.ObjectStatus
		wantErr      bool
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: object.ObjMetadataSet{},
			wantErr:   true,
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
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()

			invClient, err := NewUnstructuredClient(tf,
				configMapToInventory, inventoryToConfigMap, ConfigMapGVK)
			require.NoError(t, err)
			err = invClient.Delete(context.TODO(), tc.inv, DeleteOptions{})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func ignoreErrInfoToObjMeta(info *resource.Info) object.ObjMetadata {
	objMeta, _ := object.InfoToObjMeta(info)
	return objMeta
}

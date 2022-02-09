// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestStoreCreate(t *testing.T) {
	tests := map[string]struct {
		invObjs       object.ObjMetadataSet
		localObjs     object.ObjMetadataSet
		expectedError error
	}{
		"Empty local inventory object": {
			localObjs: object.ObjMetadataSet{},
		},
		"Local inventory with a single object": {
			localObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod2),
			},
		},
		"Local inventory with multiple objects": {
			localObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
				object.UnstructuredToObjMetadata(pod2),
				object.UnstructuredToObjMetadata(pod3),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()

			tf.FakeDynamicClient.PrependReactor("get", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				ga := action.(clienttesting.GetAction)
				return true, nil, apierrors.NewNotFound(ga.GetResource().GroupResource(), ga.GetName())
			})

			var storedInventory map[string]string
			tf.FakeDynamicClient.PrependReactor("create", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				obj := *action.(clienttesting.UpdateAction).GetObject().(*unstructured.Unstructured)
				storedInventory, _, _ = unstructured.NestedStringMap(obj.Object, "data")
				return true, nil, nil
			})

			client, err := tf.DynamicClient()
			require.NoError(t, err)

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)

			invClient := &ClusterClient{
				DynamicClient: client,
				Mapper:        mapper,
				Converter:     ConfigMapConverter{},
			}

			inv := &actuation.Inventory{}
			inv.SetGroupVersionKind(inventoryObj.GroupVersionKind())
			object.DeepCopyObjectMetaInto(inventoryObj, inv)
			inv.Spec = actuation.InventorySpec{
				Objects: ObjectReferencesFromObjMetadataSet(tc.localObjs),
			}

			err = invClient.Store(inv)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
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

func TestStoreUpdate(t *testing.T) {
	tests := map[string]struct {
		localObjs object.ObjMetadataSet
		invObjs   object.ObjMetadataSet
	}{
		"Cluster and local inventories empty": {
			localObjs: object.ObjMetadataSet{},
			invObjs:   object.ObjMetadataSet{},
		},
		"Cluster and local inventories same": {
			localObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
			},
			invObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
			},
		},
		"Cluster two obj, local one": {
			localObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
			},
			invObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
				object.UnstructuredToObjMetadata(pod3),
			},
		},
		"Cluster multiple objs, local multiple different objs": {
			localObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod2),
			},
			invObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
				object.UnstructuredToObjMetadata(pod2),
				object.UnstructuredToObjMetadata(pod3),
			},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	// TODO: add support for DryRuns
	// Client and server dry-run do not throw errors.
	// invClient, err := NewInventoryConfigMap(tf, WrapInventoryObj, InvInfoToConfigMap)
	// require.NoError(t, err)
	// err = invClient.Replace(copyInventory(), object.ObjMetadataSet{}, common.DryRunClient)
	// if err != nil {
	// 	t.Fatalf("unexpected error received: %s", err)
	// }
	// err = invClient.Replace(copyInventory(), object.ObjMetadataSet{}, common.DryRunServer)
	// if err != nil {
	// 	t.Fatalf("unexpected error received: %s", err)
	// }

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf.FakeDynamicClient.PrependReactor("get", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				obj := inventoryObj.DeepCopy()
				err = unstructured.SetNestedStringMap(obj.Object, tc.invObjs.ToStringMap(), "data")
				if err != nil {
					return true, nil, err
				}
				return true, obj, nil
			})

			var storedInventory map[string]string
			tf.FakeDynamicClient.PrependReactor("update", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				obj := *action.(clienttesting.UpdateAction).GetObject().(*unstructured.Unstructured)
				storedInventory, _, _ = unstructured.NestedStringMap(obj.Object, "data")
				return true, nil, nil
			})

			client, err := tf.DynamicClient()
			require.NoError(t, err)

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)

			invClient := &ClusterClient{
				DynamicClient: client,
				Mapper:        mapper,
				Converter:     ConfigMapConverter{},
			}

			inv := &actuation.Inventory{}
			inv.SetGroupVersionKind(inventoryObj.GroupVersionKind())
			object.DeepCopyObjectMetaInto(inventoryObj, inv)
			inv.Spec = actuation.InventorySpec{
				Objects: ObjectReferencesFromObjMetadataSet(tc.localObjs),
			}

			err = invClient.Store(inv)
			require.NoError(t, err)

			expectedInventory := tc.localObjs.ToStringMap()
			// handle empty inventories special to avoid problems with empty vs nil maps
			if len(expectedInventory) != 0 || len(storedInventory) != 0 {
				assert.Equal(t, expectedInventory, storedInventory)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := map[string]struct {
		invInfo InventoryInfo
		invObjs object.ObjMetadataSet
		isError bool
	}{
		"Empty inventory info is error": {
			invInfo: InventoryInfo{},
			invObjs: object.ObjMetadataSet{},
			isError: true,
		},
		"No cluster objs": {
			invInfo: InventoryInfoFromObject(inventoryObj),
			invObjs: object.ObjMetadataSet{},
		},
		"Single cluster obj": {
			invInfo: InventoryInfoFromObject(inventoryObj),
			invObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
			},
		},
		"Multiple cluster objs": {
			invInfo: InventoryInfoFromObject(inventoryObj),
			invObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
				object.UnstructuredToObjMetadata(pod3),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()
			tf.FakeDynamicClient.PrependReactor("get", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
				obj := inventoryObj.DeepCopy()
				err := unstructured.SetNestedStringMap(obj.Object, tc.invObjs.ToStringMap(), "data")
				if err != nil {
					return true, nil, err
				}
				return true, obj, err
			})

			client, err := tf.DynamicClient()
			require.NoError(t, err)

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)

			invClient := &ClusterClient{
				DynamicClient: client,
				Mapper:        mapper,
				Converter:     ConfigMapConverter{},
			}

			inv, err := invClient.Load(tc.invInfo)
			if tc.isError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			invObjs := ObjMetadataSetFromObjectReferences(inv.Spec.Objects)
			assert.Equal(t, tc.invObjs, invObjs)
		})
	}
}

func TestDelete(t *testing.T) {
	tests := map[string]struct {
		invInfo   InventoryInfo
		localObjs object.ObjMetadataSet
		isError   bool
	}{
		"Empty inventory info is error": {
			invInfo:   InventoryInfo{},
			localObjs: object.ObjMetadataSet{},
			isError:   true,
		},
		"Empty local inventory object": {
			invInfo:   InventoryInfoFromObject(inventoryObj),
			localObjs: object.ObjMetadataSet{},
		},
		"Local inventory with a single object": {
			invInfo: InventoryInfoFromObject(inventoryObj),
			localObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod2),
			},
		},
		"Local inventory with multiple objects": {
			invInfo: InventoryInfoFromObject(inventoryObj),
			localObjs: object.ObjMetadataSet{
				object.UnstructuredToObjMetadata(pod1),
				object.UnstructuredToObjMetadata(pod2),
				object.UnstructuredToObjMetadata(pod3),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
			defer tf.Cleanup()

			mapper, err := tf.ToRESTMapper()
			require.NoError(t, err)

			var deletedIds object.ObjMetadataSet
			tf.FakeDynamicClient.PrependReactor("delete", "configmaps", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				deleteAction := action.(clienttesting.DeleteAction)
				gvk, err := mapper.KindFor(deleteAction.GetResource())
				if err != nil {
					return false, nil, err
				}
				deletedIds = append(deletedIds, object.ObjMetadata{
					GroupKind: gvk.GroupKind(),
					Name:      deleteAction.GetName(),
					Namespace: deleteAction.GetNamespace(),
				})
				return true, nil, nil
			})

			client, err := tf.DynamicClient()
			require.NoError(t, err)

			invClient := &ClusterClient{
				DynamicClient: client,
				Mapper:        mapper,
				Converter:     ConfigMapConverter{},
			}

			err = invClient.Delete(tc.invInfo)
			if tc.isError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			expectedIds := object.ObjMetadataSet{
				ObjMetadataFromObjectReference(tc.invInfo.ObjectReference),
			}
			assert.Equal(t, expectedIds, deletedIds)
		})
	}
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func TestGetClusterInventoryInfo(t *testing.T) {
	tests := map[string]struct {
		inv       *resource.Info
		localObjs []object.ObjMetadata
		isError   bool
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: []object.ObjMetadata{},
			isError:   true,
		},
		"Empty local inventory object": {
			inv:       invInfo,
			localObjs: []object.ObjMetadata{},
			isError:   false,
		},
		"Local inventory with a single object": {
			inv: invInfo,
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			isError: false,
		},
		"Local inventory with multiple objects": {
			inv: invInfo,
			localObjs: []object.ObjMetadata{
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
			invClient, _ := NewInventoryClient(tf)
			fakeBuilder := FakeBuilder{}
			fakeBuilder.SetInventoryObjs(tc.localObjs)
			invClient.builderFunc = fakeBuilder.GetBuilder()
			var inv *resource.Info
			if tc.inv != nil {
				inv = storeObjsInInventory(tc.inv, tc.localObjs)
			}
			clusterInv, err := invClient.getClusterInventoryInfo(inv)
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
				if !object.SetEquals(tc.localObjs, clusterObjs) {
					t.Fatalf("expected cluster objs (%v), got (%v)", tc.localObjs, clusterObjs)
				}
			}
		})
	}
}

func TestMerge(t *testing.T) {
	tests := map[string]struct {
		localInv    *resource.Info
		localObjs   []object.ObjMetadata
		clusterObjs []object.ObjMetadata
		pruneObjs   []object.ObjMetadata
		isError     bool
	}{
		"Nil local inventory object is error": {
			localInv:    nil,
			localObjs:   []object.ObjMetadata{},
			clusterObjs: []object.ObjMetadata{},
			pruneObjs:   []object.ObjMetadata{},
			isError:     true,
		},
		"Cluster and local inventories empty: no prune objects; no change": {
			localInv:    copyInventoryInfo(),
			localObjs:   []object.ObjMetadata{},
			clusterObjs: []object.ObjMetadata{},
			pruneObjs:   []object.ObjMetadata{},
			isError:     false,
		},
		"Cluster and local inventories same: no prune objects; no change": {
			localInv: copyInventoryInfo(),
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			pruneObjs: []object.ObjMetadata{},
			isError:   false,
		},
		"Cluster two obj, local one: prune obj": {
			localInv: copyInventoryInfo(),
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			pruneObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod3Info),
			},
			isError: false,
		},
		"Cluster multiple objs, local multiple different objs: prune objs": {
			localInv: copyInventoryInfo(),
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			pruneObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			isError: false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				// Create the local inventory object storing "tc.localObjs"
				invClient, _ := NewInventoryClient(tf)
				invClient.SetDryRunStrategy(drs)
				// Create a fake builder to return "tc.clusterObjs" from
				// the cluster inventory object.
				fakeBuilder := FakeBuilder{}
				fakeBuilder.SetInventoryObjs(tc.clusterObjs)
				invClient.builderFunc = fakeBuilder.GetBuilder()
				// Call "Merge" to create the union of clusterObjs and localObjs.
				pruneObjs, err := invClient.Merge(tc.localInv, tc.localObjs)
				if tc.isError {
					if err == nil {
						t.Fatalf("expected error but received none")
					}
					return
				}
				if !tc.isError && err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if !object.SetEquals(tc.pruneObjs, pruneObjs) {
					t.Errorf("expected (%v) prune objs; got (%v)", tc.pruneObjs, pruneObjs)
				}
			})
		}
	}
}

func TestCreateInventory(t *testing.T) {
	tests := map[string]struct {
		inv       *resource.Info
		localObjs []object.ObjMetadata
		isError   bool
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: []object.ObjMetadata{},
			isError:   true,
		},
		"Empty local inventory object": {
			inv:       invInfo,
			localObjs: []object.ObjMetadata{},
			isError:   false,
		},
		"Local inventory with a single object": {
			inv: invInfo,
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			isError: false,
		},
		"Local inventory with multiple objects": {
			inv: invInfo,
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			isError: false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	// The fake client must see a POST to the confimap URL.
	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && cmPathRegex.Match([]byte(req.URL.Path)) {
				b, err := ioutil.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				cm := corev1.ConfigMap{}
				err = runtime.DecodeInto(codec, b, &cm)
				if err != nil {
					return nil, err
				}
				bodyRC := ioutil.NopCloser(bytes.NewReader(b))
				return &http.Response{StatusCode: http.StatusCreated, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, nil
			}
			return nil, nil
		}),
	}
	tf.ClientConfigVal = cmdtesting.DefaultClientConfig()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, _ := NewInventoryClient(tf)
			inv := tc.inv
			if inv != nil {
				inv = storeObjsInInventory(tc.inv, tc.localObjs)
			}
			err := invClient.createInventoryObj(inv)
			if !tc.isError && err != nil {
				t.Fatalf("unexpected error received: %s", err)
			}
			if tc.isError && err == nil {
				t.Fatalf("expected error but received none")
			}
		})
	}
}

func TestReplace(t *testing.T) {
	tests := map[string]struct {
		localInv    *resource.Info
		localObjs   []object.ObjMetadata
		clusterObjs []object.ObjMetadata
		isError     bool
	}{
		"Local inventory nil is error": {
			localInv:    nil,
			localObjs:   []object.ObjMetadata{},
			clusterObjs: []object.ObjMetadata{},
			isError:     true,
		},
		"Cluster and local inventories empty: no error": {
			localInv:    copyInventoryInfo(),
			localObjs:   []object.ObjMetadata{},
			clusterObjs: []object.ObjMetadata{},
			isError:     false,
		},
		"Cluster and local inventories same: no error": {
			localInv: copyInventoryInfo(),
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			isError: false,
		},
		"Cluster two obj, local one: no error": {
			localInv: copyInventoryInfo(),
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
			},
			clusterObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod3Info),
			},
			isError: false,
		},
		"Cluster multiple objs, local multiple different objs: no error": {
			localInv: copyInventoryInfo(),
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod2Info),
			},
			clusterObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
			isError: false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				invClient, _ := NewInventoryClient(tf)
				invClient.SetDryRunStrategy(drs)
				// Create fake builder returning the cluster inventory object
				// storing the "tc.clusterObjs" objects.
				fakeBuilder := FakeBuilder{}
				fakeBuilder.SetInventoryObjs(tc.clusterObjs)
				invClient.builderFunc = fakeBuilder.GetBuilder()
				// Call "Replace", passing in the new local inventory objects
				err := invClient.Replace(tc.localInv, tc.localObjs)
				if tc.isError {
					if err == nil {
						t.Fatalf("expected error but received none")
					}
					return
				}
				if !tc.isError && err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
			})
		}
	}
}

func TestGetClusterObjs(t *testing.T) {
	tests := map[string]struct {
		localInv    *resource.Info
		clusterObjs []object.ObjMetadata
		isError     bool
	}{
		"Nil cluster inventory is error": {
			localInv:    nil,
			clusterObjs: []object.ObjMetadata{},
			isError:     true,
		},
		"No cluster objs": {
			localInv:    copyInventoryInfo(),
			clusterObjs: []object.ObjMetadata{},
			isError:     false,
		},
		"Single cluster obj": {
			localInv:    copyInventoryInfo(),
			clusterObjs: []object.ObjMetadata{ignoreErrInfoToObjMeta(pod1Info)},
			isError:     false,
		},
		"Multiple cluster objs": {
			localInv:    copyInventoryInfo(),
			clusterObjs: []object.ObjMetadata{ignoreErrInfoToObjMeta(pod1Info), ignoreErrInfoToObjMeta(pod3Info)},
			isError:     false,
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, _ := NewInventoryClient(tf)
			// Create fake builder returning "tc.clusterObjs" from cluster inventory.
			fakeBuilder := FakeBuilder{}
			fakeBuilder.SetInventoryObjs(tc.clusterObjs)
			invClient.builderFunc = fakeBuilder.GetBuilder()
			// Call "GetClusterObjs" and compare returned cluster inventory objs to expected.
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
			if !object.SetEquals(tc.clusterObjs, clusterObjs) {
				t.Errorf("expected (%v) cluster inventory objs; got (%v)", tc.clusterObjs, clusterObjs)
			}
		})
	}
}

func TestDeleteInventoryObj(t *testing.T) {
	tests := map[string]struct {
		inv       *resource.Info
		localObjs []object.ObjMetadata
	}{
		"Nil local inventory object is an error": {
			inv:       nil,
			localObjs: []object.ObjMetadata{},
		},
		"Empty local inventory object": {
			inv:       invInfo,
			localObjs: []object.ObjMetadata{},
		},
		"Local inventory with a single object": {
			inv: invInfo,
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod2Info),
			},
		},
		"Local inventory with multiple objects": {
			inv: invInfo,
			localObjs: []object.ObjMetadata{
				ignoreErrInfoToObjMeta(pod1Info),
				ignoreErrInfoToObjMeta(pod2Info),
				ignoreErrInfoToObjMeta(pod3Info)},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	tf.UnstructuredClient = &fake.RESTClient{
		NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.Method == "DELETE" && cmPathRegex.Match([]byte(req.URL.Path)) {
				b, err := ioutil.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				cm := corev1.ConfigMap{}
				err = runtime.DecodeInto(codec, b, &cm)
				if err != nil {
					return nil, err
				}
				bodyRC := ioutil.NopCloser(bytes.NewReader(b))
				return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: bodyRC}, nil
			}
			return nil, nil
		}),
	}
	tf.ClientConfigVal = cmdtesting.DefaultClientConfig()

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				invClient, _ := NewInventoryClient(tf)
				invClient.SetDryRunStrategy(drs)
				inv := tc.inv
				if inv != nil {
					inv = storeObjsInInventory(tc.inv, tc.localObjs)
				}
				err := invClient.DeleteInventoryObj(inv)
				if err != nil {
					t.Fatalf("unexpected error received: %s", err)
				}
			})
		}
	}
}

func TestClearInventoryObject(t *testing.T) {
	pod1 := ignoreErrInfoToObjMeta(pod1Info)
	pod3 := ignoreErrInfoToObjMeta(pod3Info)
	inv := storeObjsInInventory(invInfo, []object.ObjMetadata{pod1, pod3})
	tests := map[string]struct {
		invInfo *resource.Info
		isError bool
	}{
		"Nil info should error": {
			invInfo: nil,
			isError: true,
		},
		"Info with nil Object should error": {
			invInfo: nilInfo,
			isError: true,
		},
		"Single non-inventory object should error": {
			invInfo: pod1Info,
			isError: true,
		},
		"Single inventory object without data should stay cleared": {
			invInfo: invInfo,
			isError: false,
		},
		"Single inventory object with data should be cleared": {
			invInfo: inv,
			isError: false,
		},
	}
	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			invClient, _ := NewInventoryClient(tf)
			invInfo, err := invClient.ClearInventoryObj(tc.invInfo)
			if tc.isError {
				if err == nil {
					t.Errorf("Should have produced an error, but returned none.")
				}
			}
			if !tc.isError {
				if err != nil {
					t.Fatalf("Received unexpected error: %s", err)
				}
				wrapped := WrapInventoryObj(invInfo)
				objs, err := wrapped.Load()
				if err != nil {
					t.Fatalf("Received unexpected error: %s", err)
				}
				if len(objs) > 0 {
					t.Errorf("Inventory object inventory not cleared: %#v\n", objs)
				}
			}
		})
	}
}

type invAndObjs struct {
	inv     *resource.Info
	invObjs []object.ObjMetadata
}

func TestMergeInventoryObjs(t *testing.T) {
	pod1Obj := ignoreErrInfoToObjMeta(pod1Info)
	pod2Obj := ignoreErrInfoToObjMeta(pod2Info)
	pod3Obj := ignoreErrInfoToObjMeta(pod3Info)
	tests := map[string]struct {
		invs     []invAndObjs
		expected []object.ObjMetadata
	}{
		"Single inventory object with no inventory is valid": {
			invs: []invAndObjs{
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{},
				},
			},
			expected: []object.ObjMetadata{},
		},
		"Single inventory object returns same objects": {
			invs: []invAndObjs{
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod1Obj},
				},
			},
			expected: []object.ObjMetadata{pod1Obj},
		},
		"Two inventories with the same objects returns them": {
			invs: []invAndObjs{
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod1Obj},
				},
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod1Obj},
				},
			},
			expected: []object.ObjMetadata{pod1Obj},
		},
		"Two inventories with different retain the union": {
			invs: []invAndObjs{
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod1Obj},
				},
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod2Obj},
				},
			},
			expected: []object.ObjMetadata{pod1Obj, pod2Obj},
		},
		"More than two inventory objects retains all objects": {
			invs: []invAndObjs{
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod1Obj, pod2Obj},
				},
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod2Obj},
				},
				{
					inv:     copyInventoryInfo(),
					invObjs: []object.ObjMetadata{pod3Obj},
				},
			},
			expected: []object.ObjMetadata{pod1Obj, pod2Obj, pod3Obj},
		},
	}

	tf := cmdtesting.NewTestFactory().WithNamespace(testNamespace)
	defer tf.Cleanup()

	for name, tc := range tests {
		for i := range common.Strategies {
			drs := common.Strategies[i]
			t.Run(name, func(t *testing.T) {
				invClient, _ := NewInventoryClient(tf)
				invClient.SetDryRunStrategy(drs)
				inventories := []*resource.Info{}
				for _, i := range tc.invs {
					inv := storeObjsInInventory(i.inv, i.invObjs)
					inventories = append(inventories, inv)
				}
				retained, err := invClient.mergeClusterInventory(inventories)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				wrapped := WrapInventoryObj(retained)
				mergedObjs, _ := wrapped.Load()
				if !object.SetEquals(tc.expected, mergedObjs) {
					t.Errorf("expected merged inventory objects (%v), got (%v)", tc.expected, mergedObjs)
				}
			})
		}
	}
}

func ignoreErrInfoToObjMeta(info *resource.Info) object.ObjMetadata {
	objMeta, _ := object.InfoToObjMeta(info)
	return objMeta
}

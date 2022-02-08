// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

var podName = "pod-1"
var pdbName = "pdb"

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
			"uid":       "pod-uid",
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

var crontabCRManifest = `
apiVersion: "stable.example.com/v1"
kind: CronTab
metadata:
  name: cron-tab-01
  namespace: test-namespace
`

func TestGetSpecObjects(t *testing.T) {
	invTypeMeta := v1.TypeMeta{
		APIVersion: inventoryObj.GetAPIVersion(),
		Kind:       inventoryObj.GetKind(),
	}
	invObjMeta := v1.ObjectMeta{
		Name:      inventoryObj.GetName(),
		Namespace: inventoryObj.GetNamespace(),
		Labels:    inventoryObj.GetLabels(),
	}

	tests := map[string]struct {
		clusterObjs  []runtime.Object
		invObjs      object.UnstructuredSet
		expectedObjs object.UnstructuredSet
	}{
		"empty inventory": {
			clusterObjs:  []runtime.Object{},
			invObjs:      object.UnstructuredSet{},
			expectedObjs: nil,
		},
		"three objects": {
			clusterObjs:  []runtime.Object{pod, pdb, namespace},
			invObjs:      object.UnstructuredSet{pod, pdb, namespace},
			expectedObjs: object.UnstructuredSet{pod, pdb, namespace},
		},
		"skip unrecognized type": {
			clusterObjs:  []runtime.Object{pdb, namespace},
			invObjs:      object.UnstructuredSet{testutil.Unstructured(t, crontabCRManifest), pdb, namespace},
			expectedObjs: object.UnstructuredSet{pdb, namespace},
		},
		"skip not found": {
			clusterObjs:  []runtime.Object{namespace},
			invObjs:      object.UnstructuredSet{pdb, namespace},
			expectedObjs: object.UnstructuredSet{namespace},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// store object schemas in mapper
			mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme.Scheme,
				scheme.Scheme.PrioritizedVersionsAllGroups()...)

			// store objects in cluster
			dynamicClient := fake.NewSimpleDynamicClient(scheme.Scheme, tc.clusterObjs...)

			// store objects in inventory
			invObjIds := object.UnstructuredSetToObjMetadataSet(tc.invObjs)
			inv := &actuation.Inventory{
				TypeMeta:   invTypeMeta,
				ObjectMeta: invObjMeta,
				Spec: actuation.InventorySpec{
					Objects: ObjectReferencesFromObjMetadataSet(invObjIds),
				},
			}

			// get inventory spec objects from cluster
			invObjManager := &ObjectManager{
				Mapper:        mapper,
				DynamicClient: dynamicClient,
			}
			objs, err := invObjManager.GetSpecObjects(context.TODO(), inv)
			assert.NoError(t, err)
			testutil.AssertEqual(t, tc.expectedObjs, objs)
		})
	}
}

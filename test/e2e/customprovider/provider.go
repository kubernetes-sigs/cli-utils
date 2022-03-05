// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package customprovider

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
)

var InventoryCRD = []byte(strings.TrimSpace(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: inventories.cli-utils.example.io
spec:
  conversion:
    strategy: None
  group: cli-utils.example.io
  names:
    kind: Inventory
    listKind: InventoryList
    plural: inventories
    singular: inventory
  scope: Namespaced
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: Example for cli-utils e2e tests
        properties:
          apiVersion:
            type: string
          kind:
            type: string
          metadata:
            type: object
          spec:
            properties:
              objects:
                items:
                  properties:
                    group:
                      type: string
                    kind:
                      type: string
                    name:
                      type: string
                    namespace:
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  type: object
                type: array
            type: object
          status:
            properties:
              objects:
                items:
                  properties:
                    group:
                      type: string
                    kind:
                      type: string
                    name:
                      type: string
                    namespace:
                      type: string
                    strategy:
                      type: string
                    actuation:
                      type: string
                    reconcile:
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  - strategy
                  - actuation
                  - reconcile
                  type: object
                type: array
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
`))

var InventoryGVK = schema.GroupVersionKind{
	Group:   "cli-utils.example.io",
	Version: "v1alpha1",
	Kind:    "Inventory",
}

var _ inventory.ClientFactory = CustomClientFactory{}

type CustomClientFactory struct {
}

func (CustomClientFactory) NewClient(factory util.Factory) (inventory.Client, error) {
	return inventory.NewClient(factory,
		WrapInventoryObj, invToUnstructuredFunc, inventory.StatusPolicyAll)
}

func invToUnstructuredFunc(inv inventory.Info) *unstructured.Unstructured {
	switch invInfo := inv.(type) {
	case *InventoryCustomType:
		return invInfo.inv
	default:
		return nil
	}
}

func WrapInventoryObj(obj *unstructured.Unstructured) inventory.Storage {
	return &InventoryCustomType{inv: obj}
}

func WrapInventoryInfoObj(obj *unstructured.Unstructured) inventory.Info {
	return &InventoryCustomType{inv: obj}
}

var _ inventory.Storage = &InventoryCustomType{}
var _ inventory.Info = &InventoryCustomType{}

type InventoryCustomType struct {
	inv *unstructured.Unstructured
}

func (i InventoryCustomType) Namespace() string {
	return i.inv.GetNamespace()
}

func (i InventoryCustomType) Name() string {
	return i.inv.GetName()
}

func (i InventoryCustomType) Strategy() inventory.Strategy {
	return inventory.NameStrategy
}

func (i InventoryCustomType) ID() string {
	labels := i.inv.GetLabels()
	id, found := labels[common.InventoryLabel]
	if !found {
		return ""
	}
	return id
}

func (i InventoryCustomType) Load() (object.ObjMetadataSet, error) {
	var inv object.ObjMetadataSet
	s, found, err := unstructured.NestedSlice(i.inv.Object, "spec", "objects")
	if err != nil {
		return inv, err
	}
	if !found {
		return inv, nil
	}
	for _, item := range s {
		m := item.(map[string]interface{})
		namespace, _, _ := unstructured.NestedString(m, "namespace")
		name, _, _ := unstructured.NestedString(m, "name")
		group, _, _ := unstructured.NestedString(m, "group")
		kind, _, _ := unstructured.NestedString(m, "kind")
		id := object.ObjMetadata{
			Namespace: namespace,
			Name:      name,
			GroupKind: schema.GroupKind{
				Group: group,
				Kind:  kind,
			},
		}
		inv = append(inv, id)
	}
	return inv, nil
}

func (i InventoryCustomType) Store(objs object.ObjMetadataSet, status []actuation.ObjectStatus) error {
	var specObjs []interface{}
	for _, obj := range objs {
		specObjs = append(specObjs, map[string]interface{}{
			"group":     obj.GroupKind.Group,
			"kind":      obj.GroupKind.Kind,
			"namespace": obj.Namespace,
			"name":      obj.Name,
		})
	}
	var statusObjs []interface{}
	for _, objStatus := range status {
		statusObjs = append(statusObjs, map[string]interface{}{
			"group":     objStatus.Group,
			"kind":      objStatus.Kind,
			"namespace": objStatus.Namespace,
			"name":      objStatus.Name,
			"strategy":  objStatus.Strategy.String(),
			"actuation": objStatus.Actuation.String(),
			"reconcile": objStatus.Reconcile.String(),
		})
	}
	if len(specObjs) > 0 {
		err := unstructured.SetNestedSlice(i.inv.Object, specObjs, "spec", "objects")
		if err != nil {
			return err
		}
	} else {
		unstructured.RemoveNestedField(i.inv.Object, "spec")
	}
	if len(statusObjs) > 0 {
		err := unstructured.SetNestedSlice(i.inv.Object, statusObjs, "status", "objects")
		if err != nil {
			return err
		}
	} else {
		unstructured.RemoveNestedField(i.inv.Object, "status")
	}
	return nil
}

func (i InventoryCustomType) GetObject() (*unstructured.Unstructured, error) {
	return i.inv, nil
}

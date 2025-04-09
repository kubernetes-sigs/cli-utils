// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package customprovider

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubectl/pkg/cmd/util"
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
	return inventory.NewUnstructuredClient(factory, fromUnstructured, toUnstructured, toUnstructuredStatus, InventoryGVK)
}

func toUnstructured(uObj *unstructured.Unstructured, inv *inventory.SingleObjectInventory) (*unstructured.Unstructured, error) {
	// Populate the spec
	var specObjs []interface{}
	for _, obj := range inv.ObjectRefs {
		specObjs = append(specObjs, map[string]interface{}{
			"group":     obj.GroupKind.Group,
			"kind":      obj.GroupKind.Kind,
			"namespace": obj.Namespace,
			"name":      obj.Name,
		})
	}
	if len(specObjs) > 0 {
		err := unstructured.SetNestedSlice(uObj.Object, specObjs, "spec", "objects")
		if err != nil {
			return nil, err
		}
	} else {
		unstructured.RemoveNestedField(uObj.Object, "spec")
	}
	// Remove the status. It's ignored by the server when calling Create/Update.
	unstructured.RemoveNestedField(uObj.Object, "status")
	return uObj, nil
}

func toUnstructuredStatus(uObj *unstructured.Unstructured, inv *inventory.SingleObjectInventory) (*unstructured.Unstructured, error) {
	// Remove the spec. It's ignored by the server when calling UpdateStatus.
	unstructured.RemoveNestedField(uObj.Object, "spec")
	// Populate the status.
	var statusObjs []interface{}
	for _, objStatus := range inv.ObjectStatuses {
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
	if len(statusObjs) > 0 {
		err := unstructured.SetNestedSlice(uObj.Object, statusObjs, "status", "objects")
		if err != nil {
			return nil, err
		}
	} else {
		unstructured.RemoveNestedField(uObj.Object, "status")
	}
	return uObj, nil
}

func fromUnstructured(obj *unstructured.Unstructured) (*inventory.SingleObjectInventory, error) {
	inv := inventory.NewSingleObjectInventory(obj)
	s, found, err := unstructured.NestedSlice(obj.Object, "spec", "objects")
	if err != nil {
		return nil, err
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
		inv.ObjectRefs = append(inv.ObjectRefs, id)
	}
	return inv, nil
}

func WrapInventoryInfoObj(obj *unstructured.Unstructured) (inventory.Info, error) {
	return inventory.NewSingleObjectInventory(obj).Info(), nil
}

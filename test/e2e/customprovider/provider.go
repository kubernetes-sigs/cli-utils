// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package customprovider

import (
	"fmt"
	"strings"

	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/inventory"
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

var _ inventory.ClientFactory = CustomClientFactory{}

type CustomClientFactory struct {
}

func (CustomClientFactory) NewClient(factory util.Factory) (inventory.Client, error) {
	client, err := factory.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("error getting dynamic client: %v", err)
	}

	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("error getting rest mapper: %v", err)
	}
	return &inventory.ClusterClient{
		DynamicClient: client,
		Mapper:        mapper,
		Converter:     CustomConverter{},
		StatusPolicy:  inventory.StatusPolicyAll,
	}, nil
}

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package flagutils

import (
	"fmt"

	"sigs.k8s.io/cli-utils/pkg/inventory"
)

const (
	InventoryPolicyFlag   = "inventory-policy"
	InventoryPolicyStrict = "strict"
	InventoryPolicyAdopt  = "adopt"
)

func ConvertInventoryPolicy(policy string) (inventory.InventoryPolicy, error) {
	switch policy {
	case InventoryPolicyStrict:
		return inventory.InventoryPolicyMustMatch, nil
	case InventoryPolicyAdopt:
		return inventory.AdoptIfNoInventory, nil
	default:
		return inventory.InventoryPolicyMustMatch, fmt.Errorf(
			"inventory policy must be one of strict, adopt")
	}
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package configmap

// Template for ConfigMap inventory object. The following fields
// must be filled in for this to be valid:
//
//  <DATETIME>: The time this is auto-generated
//  <NAMESPACE>: The namespace to place this inventory object
//  <RANDOMSUFFIX>: The random suffix added to the end of the name
//  <INVENTORYID>: The label value to retrieve this inventory object
//
const ConfigMapTemplate = `# NOTE: auto-generated. Some fields should NOT be modified.
# Date: <DATETIME>
#
# Contains the "inventory object" template ConfigMap.
# When this object is applied, it is handled specially,
# storing the metadata of all the other objects applied.
# This object and its stored inventory is subsequently
# used to calculate the set of objects to automatically
# delete (prune), when an object is omitted from further
# applies. When applied, this "inventory object" is also
# used to identify the entire set of objects to delete.
#
# NOTE: The name of this inventory template file
# does NOT have any impact on group-related functionality
# such as deletion or pruning.
#
apiVersion: v1
kind: ConfigMap
metadata:
  # DANGER: Do not change the inventory object namespace.
  # Changing the namespace will cause a loss of continuity
  # with previously applied grouped objects. Set deletion
  # and pruning functionality will be impaired.
  namespace: <NAMESPACE>
  # NOTE: The name of the inventory object does NOT have
  # any impact on group-related functionality such as
  # deletion or pruning.
  name: inventory-<RANDOMSUFFIX>
  labels:
    # DANGER: Do not change the value of this label.
    # Changing this value will cause a loss of continuity
    # with previously applied grouped objects. Set deletion
    # and pruning functionality will be impaired.
    cli-utils.sigs.k8s.io/inventory-id: <INVENTORYID>
`

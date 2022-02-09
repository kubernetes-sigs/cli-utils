// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apis/actuation"
	"sigs.k8s.io/cli-utils/pkg/common"
)

type InMemoryClient struct {
	store map[InventoryInfo]*actuation.Inventory
}

var _ Client = &InMemoryClient{}

func (imc *InMemoryClient) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "actuation",
		Version: "v1",
		Kind:    "InMemoryClient",
	}
}

func (imc *InMemoryClient) Load(_ context.Context, invInfo InventoryInfo) (*actuation.Inventory, error) {
	klog.V(4).Infof("loading inventory in memory: %v", NewInfoStringer(invInfo))

	imc.lazyInit()
	if inv, ok := imc.store[invInfo]; ok {
		return inv.DeepCopy(), nil
	}
	// TODO: Should return be NotFound error or just nil?
	return nil, nil
}

func (imc *InMemoryClient) Store(_ context.Context, inv *actuation.Inventory, dryRun common.DryRunStrategy) error {
	if inv == nil {
		return errors.New("inventory must not be nil")
	}
	invInfo := InventoryInfoFromObject(inv)

	if dryRun.ClientOrServerDryRun() {
		klog.V(4).Infoln("storing object in memory (dry-run): %v", NewInfoStringer(invInfo))
		return nil
	}
	klog.V(4).Infof("storing object in memory: %v", NewInfoStringer(invInfo))

	imc.lazyInit()
	imc.store[invInfo] = inv.DeepCopy()
	return nil
}

func (imc *InMemoryClient) Delete(_ context.Context, invInfo InventoryInfo, dryRun common.DryRunStrategy) error {
	if dryRun.ClientOrServerDryRun() {
		klog.V(4).Infoln("deleting object in memory (dry-run): %v", NewInfoStringer(invInfo))
		return nil
	}
	klog.V(4).Infof("deleting object in memory: %v", NewInfoStringer(invInfo))

	imc.lazyInit()
	delete(imc.store, invInfo)
	return nil
}

func (imc *InMemoryClient) lazyInit() {
	if imc.store == nil {
		imc.store = make(map[InventoryInfo]*actuation.Inventory)
	}
}

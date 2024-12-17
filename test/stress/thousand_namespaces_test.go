// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stress

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// thousandNamespacesTest tests a CRD and 1,000 new namespaces, each with
// 1 ConfigMap and 1 CronTab in it. This uses implicit dependencies and many
// namespaces with custom resources (json storage) as well and built-in
// resources (proto storage).
//
// With the StatusWatcher, this should only needs FOUR root-scoped informers
// (CRD, Namespace, ConfigMap, CronTab), For comparison, the StatusPoller used
// 2,002 LISTs for each attempt (two root-scoped and two namespace-scoped per
// namespace).
func thousandNamespacesTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply LOTS of resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	crdObj := e2eutil.ManifestToUnstructured([]byte(cronTabCRDYaml))

	resources := []*unstructured.Unstructured{crdObj}

	namespaceObjTemplate := e2eutil.ManifestToUnstructured([]byte(namespaceYaml))
	namespaceObjTemplate.SetLabels(map[string]string{e2eutil.TestIDLabel: inventoryID})

	configMapObjTemplate := e2eutil.ManifestToUnstructured([]byte(configMapYaml))
	configMapObjTemplate.SetLabels(map[string]string{e2eutil.TestIDLabel: inventoryID})

	cronTabObjTemplate := e2eutil.ManifestToUnstructured([]byte(cronTabYaml))
	cronTabObjTemplate.SetLabels(map[string]string{e2eutil.TestIDLabel: inventoryID})

	objectCount := 1000

	for i := 1; i <= objectCount; i++ {
		ns := fmt.Sprintf("%s-%d", namespaceName, i)
		namespaceObj := namespaceObjTemplate.DeepCopy()
		namespaceObj.SetName(ns)
		resources = append(resources, namespaceObj)

		configMapObj := configMapObjTemplate.DeepCopy()
		configMapObj.SetName(fmt.Sprintf("configmap-%d", i))
		configMapObj.SetNamespace(ns)
		resources = append(resources, configMapObj)

		cronTabObj := cronTabObjTemplate.DeepCopy()
		cronTabObj.SetName(fmt.Sprintf("crontab-%d", i))
		cronTabObj.SetNamespace(ns)
		resources = append(resources, cronTabObj)
	}

	defer func() {
		// Can't delete custom resources if the CRD is still terminating
		if e2eutil.UnstructuredExistsAndIsNotTerminating(ctx, c, crdObj) {
			By("Cleanup CronTabs")
			e2eutil.DeleteAllUnstructuredIfExists(ctx, c, cronTabObjTemplate)
			By("Cleanup CRD")
			e2eutil.DeleteUnstructuredIfExists(ctx, c, crdObj)
		}

		By("Cleanup ConfigMaps")
		e2eutil.DeleteAllUnstructuredIfExists(ctx, c, configMapObjTemplate)
		By("Cleanup Namespaces")
		e2eutil.DeleteAllUnstructuredIfExists(ctx, c, namespaceObjTemplate)
	}()

	start := time.Now()

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		// SSA reduces GET+PATCH to just PATCH, which is faster
		ServerSideOptions: common.ServerSideOptions{
			ServerSideApply: true,
			ForceConflicts:  true,
			FieldManager:    "cli-utils.kubernetes.io",
		},
		ReconcileTimeout: 30 * time.Minute,
		EmitStatusEvents: false,
	}))

	duration := time.Since(start)
	klog.Infof("Applier.Run execution time: %v", duration)

	for _, e := range applierEvents {
		Expect(e.ErrorEvent.Err).To(BeNil())
	}
	for _, e := range applierEvents {
		Expect(e.ApplyEvent.Error).To(BeNil(), "ApplyEvent: %v", e.ApplyEvent)
	}
	for _, e := range applierEvents {
		if e.Type == event.WaitType {
			Expect(e.WaitEvent.Status).To(BeElementOf(event.ReconcilePending, event.ReconcileSuccessful), "WaitEvent: %v", e.WaitEvent)
		}
	}

	By("Verify inventory created")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, len(resources), len(resources))

	By("Verify CRD created")
	e2eutil.AssertUnstructuredExists(ctx, c, crdObj)

	By(fmt.Sprintf("Verify %d Namespaces created", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, namespaceObjTemplate, objectCount)

	By(fmt.Sprintf("Verify %d ConfigMaps created", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, configMapObjTemplate, objectCount)

	By(fmt.Sprintf("Verify %d CronTabs created", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, cronTabObjTemplate, objectCount)

	By("Destroy LOTS of resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	start = time.Now()

	destroyerEvents := e2eutil.RunCollect(destroyer.Run(ctx, inventoryInfo, apply.DestroyerOptions{
		InventoryPolicy: inventory.PolicyAdoptIfNoInventory,
		DeleteTimeout:   30 * time.Minute,
	}))

	duration = time.Since(start)
	klog.Infof("Destroyer.Run execution time: %v", duration)

	for _, e := range destroyerEvents {
		Expect(e.ErrorEvent.Err).To(BeNil())
	}
	for _, e := range destroyerEvents {
		Expect(e.PruneEvent.Error).To(BeNil(), "PruneEvent: %v", e.PruneEvent)
	}
	for _, e := range destroyerEvents {
		if e.Type == event.WaitType {
			Expect(e.WaitEvent.Status).To(BeElementOf(event.ReconcilePending, event.ReconcileSuccessful), "WaitEvent: %v", e.WaitEvent)
		}
	}

	By("Verify inventory deleted")
	invConfig.InvNotExistsFunc(ctx, c, inventoryName, namespaceName, inventoryID)

	By(fmt.Sprintf("Verify %d CronTabs deleted", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, cronTabObjTemplate, 0)

	By(fmt.Sprintf("Verify %d ConfigMaps deleted", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, configMapObjTemplate, 0)

	By(fmt.Sprintf("Verify %d Namespaces deleted", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, namespaceObjTemplate, 0)

	By("Verify CRD deleted")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, crdObj)
}

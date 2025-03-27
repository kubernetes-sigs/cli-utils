// Copyright 2022 The Kubernetes Authors.
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
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// thousandDeploymentsTest tests one pre-existing namespace with 1,000
// Deployments in it.
//
// The Deployments themselves are easy to get status on, but with the retrieval
// of generated resource status (ReplicaSets & Pods), this becomes expensive.
func thousandDeploymentsTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply LOTS of resources")
	applier := invConfig.ApplierFactoryFunc()

	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)
	inventoryInfo, err := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)
	Expect(err).ToNot(HaveOccurred())

	resources := []*unstructured.Unstructured{}

	deploymentObjTemplate := e2eutil.ManifestToUnstructured([]byte(deploymentYaml))
	deploymentObjTemplate.SetLabels(map[string]string{e2eutil.TestIDLabel: inventoryID})

	objectCount := 1000

	for i := 1; i <= objectCount; i++ {
		deploymentObj := deploymentObjTemplate.DeepCopy()
		deploymentObj.SetNamespace(namespaceName)

		// change name & selector labels to avoid overlap between deployments
		name := fmt.Sprintf("pause-%d", i)
		deploymentObj.SetName(name)
		err := unstructured.SetNestedField(deploymentObj.Object, name, "spec", "selector", "matchLabels", "app")
		Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(deploymentObj.Object, name, "spec", "template", "metadata", "labels", "app")
		Expect(err).ToNot(HaveOccurred())

		resources = append(resources, deploymentObj)
	}

	defer func() {
		By("Cleanup Deployments")
		e2eutil.DeleteAllUnstructuredIfExists(ctx, c, deploymentObjTemplate)
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

	e2eutil.ExpectNoEventErrors(applierEvents)
	e2eutil.ExpectNoReconcileTimeouts(applierEvents)

	By("Verify inventory created")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, len(resources), len(resources))

	By(fmt.Sprintf("Verify %d Deployments created", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, deploymentObjTemplate, objectCount)

	By("Destroy LOTS of resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	start = time.Now()

	destroyerEvents := e2eutil.RunCollect(destroyer.Run(ctx, inventoryInfo, apply.DestroyerOptions{
		InventoryPolicy: inventory.PolicyAdoptIfNoInventory,
		DeleteTimeout:   30 * time.Minute,
	}))

	duration = time.Since(start)
	klog.Infof("Destroyer.Run execution time: %v", duration)

	e2eutil.ExpectNoEventErrors(destroyerEvents)
	e2eutil.ExpectNoReconcileTimeouts(destroyerEvents)

	By("Verify inventory deleted")
	invConfig.InvNotExistsFunc(ctx, c, inventoryName, namespaceName, inventoryID)

	By(fmt.Sprintf("Verify %d Deployments deleted", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, deploymentObjTemplate, 0)
}

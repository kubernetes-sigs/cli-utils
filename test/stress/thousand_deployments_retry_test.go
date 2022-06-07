// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package stress

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

// thousandDeploymentsRetryTest tests one pre-existing namespace with 1,000
// Deployments in it. The wait timeout is set too short to confirm
// reconciliation, but the apply/destroy is retried until success.
//
// The Deployments themselves are easy to get status on, but with the retrieval
// of generated resource status (ReplicaSets & Pods), this becomes expensive.
func thousandDeploymentsRetryTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply LOTS of resources")
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)

	inventoryInfo := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	resources := []*unstructured.Unstructured{}

	deploymentObjTemplate := e2eutil.ManifestToUnstructured([]byte(deploymentYaml))
	deploymentObjTemplate.SetLabels(map[string]string{e2eutil.TestIDLabel: inventoryID})

	objectCount := 1000

	for i := 1; i <= objectCount; i++ {
		deploymentObj := deploymentObjTemplate.DeepCopy()
		deploymentObj.SetNamespace(namespaceName)

		// change name & selector labels to avoid overlap between deployments
		name := fmt.Sprintf("nginx-%d", i)
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

	startTotal := time.Now()

	var applierEvents []event.Event

	maxAttempts := 15
	reconcileTimeout := 2 * time.Minute

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		start := time.Now()

		applierEvents = e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
			// SSA reduces GET+PATCH to just PATCH, which is faster
			ServerSideOptions: common.ServerSideOptions{
				ServerSideApply: true,
				ForceConflicts:  true,
				FieldManager:    "cli-utils.kubernetes.io",
			},
			ReconcileTimeout: reconcileTimeout,
			EmitStatusEvents: false,
		}))

		duration := time.Since(start)
		klog.Infof("Applier.Run execution time (attempt: %d): %v", attempt, duration)

		e2eutil.ExpectNoEventErrors(applierEvents)

		// Retry if ReconcileTimeout
		retry := false
		for _, e := range applierEvents {
			if e.Type == event.WaitType && e.WaitEvent.Status == event.ReconcileTimeout {
				retry = true
			}
		}
		if !retry {
			break
		}
	}

	durationTotal := time.Since(startTotal)
	klog.Infof("Applier.Run total execution time (attempts: %d): %v", maxAttempts, durationTotal)

	e2eutil.ExpectNoReconcileTimeouts(applierEvents)

	By("Verify inventory created")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, len(resources), len(resources))

	By(fmt.Sprintf("Verify %d Deployments created", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, deploymentObjTemplate, objectCount)

	By("Destroy LOTS of resources")
	destroyer := invConfig.DestroyerFactoryFunc()

	startTotal = time.Now()

	var destroyerEvents []event.Event

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		start := time.Now()

		destroyerEvents = e2eutil.RunCollect(destroyer.Run(ctx, inventoryInfo, apply.DestroyerOptions{
			InventoryPolicy: inventory.PolicyAdoptIfNoInventory,
			DeleteTimeout:   reconcileTimeout,
		}))

		duration := time.Since(start)
		klog.Infof("Destroyer.Run execution time (attempt: %d): %v", attempt, duration)

		e2eutil.ExpectNoEventErrors(destroyerEvents)

		// Retry if ReconcileTimeout
		retry := false
		for _, e := range applierEvents {
			if e.Type == event.WaitType && e.WaitEvent.Status == event.ReconcileTimeout {
				retry = true
			}
		}
		if !retry {
			break
		}
	}

	durationTotal = time.Since(startTotal)
	klog.Infof("Destroyer.Run total execution time (attempts: %d): %v", maxAttempts, durationTotal)

	e2eutil.ExpectNoReconcileTimeouts(applierEvents)

	By("Verify inventory deleted")
	invConfig.InvNotExistsFunc(ctx, c, inventoryName, namespaceName, inventoryID)

	By(fmt.Sprintf("Verify %d Deployments deleted", objectCount))
	e2eutil.AssertUnstructuredCount(ctx, c, deploymentObjTemplate, 0)
}

// Copyright 2021 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const v1EventTemplate = `
apiVersion: v1
involvedObject:
  apiVersion: v1
  kind: Pod
  name: pod
  namespace: {{.Namespace}}
kind: Event
message: Back-off restarting failed container
metadata:
  name: test
  namespace: {{.Namespace}}
reason: BackOff
type: Warning
`

const v1EventsEventTemplate = `
apiVersion: events.k8s.io/v1
eventTime: null
kind: Event
metadata:
  name: test
  namespace: {{.Namespace}}
note: Back-off restarting failed container
reason: BackOff
regarding:
  apiVersion: v1
  kind: Pod
  name: pod
  namespace: {{.Namespace}}
type: Warning
`

// Note this tests the scenario of "cohabitating" k8s objects (an object available via multiple apiGroups), but having the same UID.
// As of k8s 1.25 an example of such "cohabitating" kinds is Event which is available via both "v1" and "events.k8s.io/v1".
// See the full list of cohabitating resources on the storage level here:
// - https://github.com/kubernetes/kubernetes/blob/v1.25.0/pkg/kubeapiserver/default_storage_factory_builder.go#L124-L131
// We test that when the user upgrades their manifest from one cohabitated apiGroup to the other, then:
// - it should not result in object being pruned
// - object pruning should be skipped due to CurrentUIDFilter (even though a diff is found)
// - inventory should not double-track the object i.e. we should hold reference only to the object with the groupKind that was most recently applied
func currentUIDFilterTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	applier := invConfig.ApplierFactoryFunc()
	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)
	inventoryInfo := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)

	templateFields := struct{ Namespace string }{Namespace: namespaceName}
	v1Event := e2eutil.TemplateToUnstructured(v1EventTemplate, templateFields)
	v1EventsEvent := e2eutil.TemplateToUnstructured(v1EventsEventTemplate, templateFields)

	By("Apply resource with deprecated groupKind")
	resources := []*unstructured.Unstructured{
		v1Event,
	}
	err := e2eutil.Run(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{}))
	Expect(err).ToNot(HaveOccurred())

	By("Verify resource available in both apiGroups")
	objDeprecated := e2eutil.AssertUnstructuredExists(ctx, c, v1Event)
	objNew := e2eutil.AssertUnstructuredExists(ctx, c, v1EventsEvent)

	By("Verify UID matches for cohabitating resources")
	uid := objDeprecated.GetUID()
	Expect(uid).ToNot(BeEmpty())
	Expect(objDeprecated.GetUID()).To(Equal(objNew.GetUID()))

	By("Verify only 1 item in inventory")
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 1, 1)

	By("Apply resource with new groupKind")
	resources = []*unstructured.Unstructured{
		v1EventsEvent,
	}
	err = e2eutil.Run(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{}))
	Expect(err).ToNot(HaveOccurred())

	By("Verify resource still available in both apiGroups")
	objDeprecated = e2eutil.AssertUnstructuredExists(ctx, c, v1Event)
	objNew = e2eutil.AssertUnstructuredExists(ctx, c, v1EventsEvent)

	By("Verify UID matches for cohabitating resources")
	Expect(objDeprecated.GetUID()).To(Equal(objNew.GetUID()))

	By("Verify UID matches the UID from previous apply")
	Expect(objDeprecated.GetUID()).To(Equal(uid))

	By("Verify still only 1 item in inventory")
	// Expecting statusCount=2:
	//   one object applied and one prune skipped
	invConfig.InvSizeVerifyFunc(ctx, c, inventoryName, namespaceName, inventoryID, 1, 2)
}

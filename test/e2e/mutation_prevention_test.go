// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/apply/filter"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func mutationPreventionTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("Apply resources")
	applier := invConfig.ApplierFactoryFunc()

	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)
	inventoryInfo, err := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)
	Expect(err).ToNot(HaveOccurred())

	uDep := e2eutil.ManifestToUnstructured(deployment1)
	resources := []*unstructured.Unstructured{
		e2eutil.WithAnnotation(e2eutil.WithNamespace(uDep, namespaceName), common.LifecycleMutationAnnotation, common.IgnoreMutation),
	}

	e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
	}))

	By("Verify deployment created")
	obj := e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName))
	Expect(obj.GetAnnotations()[inventory.OwningInventoryKey]).To(Equal(inventoryInfo.GetID().String()))

	By("Mutate the on-cluster Deployment object replica count")
	dep := &appsv1.Deployment{}
	key := types.NamespacedName{Name: uDep.GetName(), Namespace: uDep.GetNamespace()}
	err = c.Get(ctx, key, dep)
	Expect(err).ToNot(HaveOccurred())
	Expect(*dep.Spec.Replicas).To(Equal(int32(4))) // verify original replica count
	dep.Spec.Replicas = ptr.To(int32(3))
	err = c.Update(ctx, dep)
	Expect(err).ToNot(HaveOccurred())

	By("Dry-run apply resources")

	e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
		DryRunStrategy:   common.DryRunClient,
	}))

	By("Verify deployment still exists and has the mutated replica count")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName))
	replicas, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(replicas).To(Equal(int64(3)))

	By("Apply resources")

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inventoryInfo, resources, apply.ApplierOptions{
		ReconcileTimeout: 2 * time.Minute,
	}))
	expEvents := []testutil.ExpEvent{
		{
			// InitTask
			EventType: event.InitType,
			InitEvent: &testutil.ExpInitEvent{},
		},
		{
			// InvAddTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-add-0",
				Type:      event.Started,
			},
		},
		{
			// InvAddTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-add-0",
				Type:      event.Finished,
			},
		},
		{
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-0",
				Type:      event.Started,
			},
		},
		{
			// Skip apply of Deployment
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplySkipped,
				Identifier: object.UnstructuredToObjMetadata(uDep),
				Error: &filter.AnnotationPreventedUpdateError{
					Annotation: common.LifecycleMutationAnnotation,
					Value:      common.IgnoreMutation,
				},
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-0",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Started,
			},
		},
		{
			// Deployment reconcile skipped.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSkipped,
				Identifier: object.UnstructuredToObjMetadata(uDep),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-0",
				Type:      event.Finished,
			},
		},
		{
			// InvSetTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-set-0",
				Type:      event.Started,
			},
		},
		{
			// InvSetTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "inventory-set-0",
				Type:      event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(applierEvents)).To(testutil.Equal(expEvents))

	By("Verify deployment still exists and has the mutated replica count")
	obj = e2eutil.AssertUnstructuredExists(ctx, c, e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(deployment1), namespaceName))
	replicas, found, err = unstructured.NestedInt64(obj.Object, "spec", "replicas")
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(replicas).To(Equal(int64(3)))
}

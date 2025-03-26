// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/apply"
	applyerror "sigs.k8s.io/cli-utils/pkg/apply/error"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/e2eutil"
	"sigs.k8s.io/cli-utils/test/e2e/invconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func continueOnErrorTest(ctx context.Context, c client.Client, invConfig invconfig.InventoryConfig, inventoryName, namespaceName string) {
	By("apply an invalid CRD")
	applier := invConfig.ApplierFactoryFunc()

	inventoryID := fmt.Sprintf("%s-%s", inventoryName, namespaceName)
	inv, err := invconfig.CreateInventoryInfo(invConfig, inventoryName, namespaceName, inventoryID)
	Expect(err).ToNot(HaveOccurred())

	invalidCrdObj := e2eutil.ManifestToUnstructured(invalidCrd)
	pod1Obj := e2eutil.WithNamespace(e2eutil.ManifestToUnstructured(pod1), namespaceName)
	resources := []*unstructured.Unstructured{
		invalidCrdObj,
		pod1Obj,
	}

	applierEvents := e2eutil.RunCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{}))

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
			// Apply invalidCrd fails
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplyFailed,
				Identifier: object.UnstructuredToObjMetadata(invalidCrdObj),
				Error: testutil.EqualError(
					applyerror.NewApplyRunError(
						cmdutil.AddSourceToErr(
							"creating",
							"unstructured",
							apierrors.NewInvalid(
								invalidCrdObj.GroupVersionKind().GroupKind(),
								invalidCrdObj.GetName(),
								field.ErrorList{
									&field.Error{
										Type:     field.ErrorTypeInvalid,
										Field:    "spec.versions",
										BadValue: []apiextensions.CustomResourceDefinitionVersion(nil),
										Detail:   "must have exactly one version marked as storage version",
									},
									&field.Error{
										Type:     field.ErrorTypeInvalid,
										Field:    "status.storedVersions",
										BadValue: []string(nil),
										Detail:   "must have at least one stored version",
									},
								},
							),
						),
					),
				),
			},
		},
		{
			// Create pod1
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Status:     event.ApplySuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
				Error:      nil,
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
			// Pod1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// CRD reconcile Skipped.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSkipped,
				Identifier: object.UnstructuredToObjMetadata(invalidCrdObj),
			},
		},
		{
			// Pod1 reconcile Successful.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Status:     event.ReconcileSuccessful,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
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
	receivedEvents := testutil.EventsToExpEvents(applierEvents)

	expEvents, receivedEvents = e2eutil.FilterOptionalEvents(expEvents, receivedEvents)

	// sort to allow comparison of multiple ApplyTasks in the same task group
	testutil.SortExpEvents(receivedEvents)

	Expect(receivedEvents).To(testutil.Equal(expEvents))

	By("Verify pod1 created")
	e2eutil.AssertUnstructuredExists(ctx, c, pod1Obj)

	By("Verify CRD not created")
	e2eutil.AssertUnstructuredDoesNotExist(ctx, c, invalidCrdObj)
}

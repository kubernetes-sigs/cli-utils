// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/object/dependson"
	"sigs.k8s.io/cli-utils/pkg/object/graph"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
	"sigs.k8s.io/cli-utils/pkg/object/validation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func skipInvalidTest(ctx context.Context, c client.Client, invConfig InventoryConfig, inventoryName, namespaceName string) {
	By("apply valid objects and skip invalid objects")
	applier := invConfig.ApplierFactoryFunc()

	inv := invConfig.InvWrapperFunc(invConfig.InventoryFactoryFunc(inventoryName, namespaceName, "test"))

	fields := struct{ Namespace string }{Namespace: namespaceName}
	// valid pod
	pod1Obj := withNamespace(manifestToUnstructured(pod1), namespaceName)
	// valid deployment with dependency
	deployment1Obj := withDependsOn(withNamespace(manifestToUnstructured(deployment1), namespaceName),
		fmt.Sprintf("/namespaces/%s/Pod/%s", namespaceName, pod1Obj.GetName()))
	// external/missing dependency
	pod3Obj := withDependsOn(withNamespace(manifestToUnstructured(pod3), namespaceName),
		fmt.Sprintf("/namespaces/%s/Pod/pod0", namespaceName))
	// cyclic dependency (podB)
	podAObj := templateToUnstructured(podATemplate, fields)
	// cyclic dependency (podA) & invalid source reference (dependency not in object set)
	podBObj := templateToUnstructured(invalidMutationPodBTemplate, fields)
	// missing name
	invalidPodObj := templateToUnstructured(invalidPodTemplate, fields)

	resources := []*unstructured.Unstructured{
		pod1Obj,
		deployment1Obj,
		pod3Obj,
		podAObj,
		podBObj,
		invalidPodObj,
	}

	applierEvents := runCollect(applier.Run(ctx, inv, resources, apply.ApplierOptions{
		EmitStatusEvents: false,
		ValidationPolicy: validation.SkipInvalid,
	}))

	expEvents := []testutil.ExpEvent{
		{
			// invalid pod validation error
			EventType: event.ValidationType,
			ValidationEvent: &testutil.ExpValidationEvent{
				Identifiers: object.ObjMetadataSet{
					object.UnstructuredToObjMetadata(invalidPodObj),
				},
				Error: testutil.EqualErrorString(validation.NewError(
					field.Required(field.NewPath("metadata", "name"), "name is required"),
					object.UnstructuredToObjMetadata(invalidPodObj),
				).Error()),
			},
		},
		{
			// Pod3 validation error
			EventType: event.ValidationType,
			ValidationEvent: &testutil.ExpValidationEvent{
				Identifiers: object.ObjMetadataSet{
					object.UnstructuredToObjMetadata(pod3Obj),
				},
				Error: testutil.EqualErrorString(validation.NewError(
					object.InvalidAnnotationError{
						Annotation: dependson.Annotation,
						Cause: graph.ExternalDependencyError{
							Edge: graph.Edge{
								From: object.UnstructuredToObjMetadata(pod3Obj),
								To: object.ObjMetadata{
									GroupKind: schema.GroupKind{Kind: "Pod"},
									Name:      "pod0",
									Namespace: namespaceName,
								},
							},
						},
					},
					object.UnstructuredToObjMetadata(pod3Obj),
				).Error()),
			},
		},
		{
			// PodB validation error
			EventType: event.ValidationType,
			ValidationEvent: &testutil.ExpValidationEvent{
				Identifiers: object.ObjMetadataSet{
					object.UnstructuredToObjMetadata(podBObj),
				},
				Error: testutil.EqualErrorString(validation.NewError(
					object.InvalidAnnotationError{
						Annotation: mutation.Annotation,
						Cause: graph.ExternalDependencyError{
							Edge: graph.Edge{
								From: object.UnstructuredToObjMetadata(podBObj),
								To: object.ObjMetadata{
									GroupKind: schema.GroupKind{Kind: "Pod"},
									Name:      "pod-a",
								},
							},
						},
					},
					object.UnstructuredToObjMetadata(podBObj),
				).Error()),
			},
		},
		{
			// Cyclic Dependency validation error
			EventType: event.ValidationType,
			ValidationEvent: &testutil.ExpValidationEvent{
				Identifiers: object.ObjMetadataSet{
					object.UnstructuredToObjMetadata(podAObj),
					object.UnstructuredToObjMetadata(podBObj),
				},
				Error: testutil.EqualErrorString(validation.NewError(
					graph.CyclicDependencyError{
						Edges: []graph.Edge{
							{
								From: object.UnstructuredToObjMetadata(podAObj),
								To:   object.UnstructuredToObjMetadata(podBObj),
							},
							{
								From: object.UnstructuredToObjMetadata(podBObj),
								To:   object.UnstructuredToObjMetadata(podAObj),
							},
						},
					},
					object.UnstructuredToObjMetadata(podAObj),
					object.UnstructuredToObjMetadata(podBObj),
				).Error()),
			},
		},
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
			// Apply Pod1
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-0",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
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
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// Pod1 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
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
			// ApplyTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-1",
				Type:      event.Started,
			},
		},
		{
			// Apply Deployment1
			EventType: event.ApplyType,
			ApplyEvent: &testutil.ExpApplyEvent{
				GroupName:  "apply-1",
				Operation:  event.Created,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			},
		},
		{
			// ApplyTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.ApplyAction,
				GroupName: "apply-1",
				Type:      event.Finished,
			},
		},
		{
			// WaitTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-1",
				Type:      event.Started,
			},
		},
		{
			// Deployment1 reconcile Pending.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			},
		},
		{
			// Deployment1 confirmed Current.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-1",
				Operation:  event.Reconciled,
				Identifier: object.UnstructuredToObjMetadata(deployment1Obj),
			},
		},
		{
			// WaitTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.WaitAction,
				GroupName: "wait-1",
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

	By("verify pod1 created and ready")
	result := assertUnstructuredExists(ctx, c, pod1Obj)
	podIP, found, err := object.NestedField(result.Object, "status", "podIP")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(podIP).NotTo(BeEmpty()) // use podIP as proxy for readiness

	By("verify deployment1 created and ready")
	result = assertUnstructuredExists(ctx, c, deployment1Obj)
	assertUnstructuredAvailable(result)

	By("verify pod3 not found")
	assertUnstructuredDoesNotExist(ctx, c, pod3Obj)

	By("verify podA not found")
	assertUnstructuredDoesNotExist(ctx, c, podAObj)

	By("verify podB not found")
	assertUnstructuredDoesNotExist(ctx, c, podBObj)

	By("modify deployment1 depends-on annotation to be invalid")
	applyUnstructured(ctx, c, withDependsOn(deployment1Obj, "invalid"))

	By("destroy valid objects and skip invalid objects")
	destroyer := invConfig.DestroyerFactoryFunc()
	destroyerEvents := runCollect(destroyer.Run(ctx, inv, apply.DestroyerOptions{
		InventoryPolicy:  inventory.AdoptIfNoInventory,
		ValidationPolicy: validation.SkipInvalid,
	}))

	expEvents = []testutil.ExpEvent{
		{
			// Deployment1 validation error
			EventType: event.ValidationType,
			ValidationEvent: &testutil.ExpValidationEvent{
				Identifiers: object.ObjMetadataSet{
					object.UnstructuredToObjMetadata(deployment1Obj),
				},
				Error: testutil.EqualErrorType(
					validation.NewError(nil), // TODO: be more specific
				),
			},
		},
		{
			// InitTask
			EventType: event.InitType,
			InitEvent: &testutil.ExpInitEvent{},
		},
		{
			// PruneTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-0",
				Type:      event.Started,
			},
		},
		// TODO: Filter deletes so dependencies don't get deleted when the objects that used to depend on them are invalid?
		{
			// Delete pod1
			EventType: event.DeleteType,
			DeleteEvent: &testutil.ExpDeleteEvent{
				GroupName:  "prune-0",
				Operation:  event.Deleted,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// PruneTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.DeleteAction,
				GroupName: "prune-0",
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
				Operation:  event.ReconcilePending,
				Identifier: object.UnstructuredToObjMetadata(pod1Obj),
			},
		},
		{
			// Pod1 confirmed NotFound.
			EventType: event.WaitType,
			WaitEvent: &testutil.ExpWaitEvent{
				GroupName:  "wait-0",
				Operation:  event.Reconciled,
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
			// DeleteInvTask start
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "delete-inventory-0",
				Type:      event.Started,
			},
		},
		{
			// DeleteInvTask finished
			EventType: event.ActionGroupType,
			ActionGroupEvent: &testutil.ExpActionGroupEvent{
				Action:    event.InventoryAction,
				GroupName: "delete-inventory-0",
				Type:      event.Finished,
			},
		},
	}
	Expect(testutil.EventsToExpEvents(destroyerEvents)).To(testutil.Equal(expEvents))

	By("verify pod1 deleted")
	assertUnstructuredDoesNotExist(ctx, c, pod1Obj)

	By("verify deployment1 not deleted")
	assertUnstructuredExists(ctx, c, deployment1Obj)
	deleteUnstructuredIfExists(ctx, c, deployment1Obj)

	By("verify pod3 not found")
	assertUnstructuredDoesNotExist(ctx, c, pod3Obj)

	By("verify podA not found")
	assertUnstructuredDoesNotExist(ctx, c, podAObj)

	By("verify podB not found")
	assertUnstructuredDoesNotExist(ctx, c, podBObj)
}

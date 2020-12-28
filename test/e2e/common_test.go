// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/object"
)

func randomString(prefix string) string {
	seed := time.Now().UTC().UnixNano()
	randomSuffix := common.RandomStr(seed)
	return fmt.Sprintf("%s%s", prefix, randomSuffix)
}

func cmInventoryManifest(name, namespace, id string) *unstructured.Unstructured {
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				common.InventoryLabel: id,
			},
		},
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{
		Object: u,
	}
}

func deploymentManifest(namespace string) *unstructured.Unstructured {
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-deployment",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func() *int32 { r := int32(4); return &r }(),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "nginx",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.19.6",
						},
					},
				},
			},
		},
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{
		Object: u,
	}
}

func updateReplicas(u *unstructured.Unstructured, replicas int) *unstructured.Unstructured {
	err := unstructured.SetNestedField(u.Object, int64(replicas), "spec", "replicas")
	if err != nil {
		panic(err)
	}
	return u
}

type expEvent struct {
	eventType event.Type

	applyEvent *expApplyEvent
}

type expApplyEvent struct {
	applyEventType event.ApplyEventType
	operation      event.ApplyEventOperation
	identifier     object.ObjMetadata
	error          error
}

func verifyEvents(expEvents []expEvent, events []event.Event) error {
	expEventIndex := 0
	for i := range events {
		e := events[i]
		ee := expEvents[expEventIndex]
		if isMatch(ee, e) {
			expEventIndex += 1
			if expEventIndex >= len(expEvents) {
				return nil
			}
		}
	}
	return fmt.Errorf("event %s not found", expEvents[expEventIndex].eventType)
}

var nilIdentifier = object.ObjMetadata{}

func isMatch(ee expEvent, e event.Event) bool {
	if ee.eventType != e.Type {
		return false
	}

	// nolint:gocritic
	switch e.Type {
	case event.ApplyType:
		aee := ee.applyEvent
		if aee == nil {
			return true
		}
		ae := e.ApplyEvent

		if aee.applyEventType != ae.Type {
			return false
		}

		if aee.applyEventType == event.ApplyEventResourceUpdate {
			if aee.operation != ae.Operation {
				return false
			}
		}

		if aee.identifier != nilIdentifier {
			if aee.identifier != ae.Identifier {
				return false
			}
		}

		if aee.error != nil {
			return ae.Error != nil
		}
		return ae.Error == nil
	}
	return true
}

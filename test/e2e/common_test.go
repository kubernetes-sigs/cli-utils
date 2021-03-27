// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func randomString(prefix string) string {
	seed := time.Now().UTC().UnixNano()
	randomSuffix := common.RandomStr(seed)
	return fmt.Sprintf("%s%s", prefix, randomSuffix)
}

func run(ch <-chan event.Event) error {
	var err error
	for e := range ch {
		if e.Type == event.ErrorType {
			err = e.ErrorEvent.Err
		}
	}
	return err
}

func runWithNoErr(ch <-chan event.Event) {
	runCollectNoErr(ch)
}

func runCollect(ch <-chan event.Event) []event.Event {
	var events []event.Event
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func runCollectNoErr(ch <-chan event.Event) []event.Event {
	events := runCollect(ch)
	for _, e := range events {
		Expect(e.Type).NotTo(Equal(event.ErrorType))
	}
	return events
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

func customInventoryManifest(name, namespace, id string) *unstructured.Unstructured {
	u := manifestToUnstructured([]byte(strings.TrimSpace(`
apiVersion: cli-utils.example.io/v1alpha1
kind: Inventory
metadata:
  name: PLACEHOLDER
`)))
	u.SetName(name)
	u.SetNamespace(namespace)
	u.SetLabels(map[string]string{
		common.InventoryLabel: id,
	})
	return u
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

func apiserviceManifest() *unstructured.Unstructured {
	apiservice := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiregistration.k8s.io/v1",
			"kind":       "APIService",
			"metadata": map[string]interface{}{
				"name": "v1beta1.custom.metrics.k8s.io",
			},
			"spec": map[string]interface{}{
				"insecureSkipTLSVerify": true,
				"group":                 "custom.metrics.k8s.io",
				"groupPriorityMinimum":  100,
				"versionPriority":       100,
				"service": map[string]interface{}{
					"name":      "custom-metrics-stackdriver-adapter",
					"namespace": "custome-metrics",
				},
				"version": "v1beta1",
			},
		},
	}
	return apiservice
}

func manifestToUnstructured(manifest []byte) *unstructured.Unstructured {
	u := make(map[string]interface{})
	err := yaml.Unmarshal(manifest, &u)
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

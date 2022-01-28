// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object/dependson"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func withReplicas(obj *unstructured.Unstructured, replicas int) *unstructured.Unstructured {
	err := unstructured.SetNestedField(obj.Object, int64(replicas), "spec", "replicas")
	Expect(err).NotTo(HaveOccurred())
	return obj
}

func withNamespace(obj *unstructured.Unstructured, namespace string) *unstructured.Unstructured {
	obj.SetNamespace(namespace)
	return obj
}

func podWithImage(obj *unstructured.Unstructured, containerName, image string) *unstructured.Unstructured {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "containers")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())

	containerFound := false
	for i := range containers {
		container := containers[i].(map[string]interface{})
		name := container["name"].(string)
		if name != containerName {
			continue
		}
		containerFound = true
		container["image"] = image
	}
	Expect(containerFound).To(BeTrue())
	err = unstructured.SetNestedSlice(obj.Object, containers, "spec", "containers")
	Expect(err).NotTo(HaveOccurred())
	return obj
}

func withNodeSelector(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	selectors, found, err := unstructured.NestedMap(obj.Object, "spec", "nodeSelector")
	Expect(err).NotTo(HaveOccurred())

	if !found {
		selectors = make(map[string]interface{})
	}
	selectors[key] = value
	err = unstructured.SetNestedMap(obj.Object, selectors, "spec", "nodeSelector")
	Expect(err).NotTo(HaveOccurred())
	return obj
}

func withAnnotation(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
	return obj
}

func withDependsOn(obj *unstructured.Unstructured, dep string) *unstructured.Unstructured {
	a := obj.GetAnnotations()
	if a == nil {
		a = make(map[string]string, 1)
	}
	a[dependson.Annotation] = dep
	obj.SetAnnotations(a)
	return obj
}

func deleteUnstructuredAndWait(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)

	err := c.Delete(ctx, obj,
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	Expect(err).NotTo(HaveOccurred(),
		"expected DELETE to not error (%s): %s", ref, err)

	waitForDeletion(ctx, c, obj)
}

func waitForDeletion(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	timeout := 30 * time.Second
	retry := 2 * time.Second

	t := time.NewTimer(timeout)
	s := time.NewTimer(0)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			Fail("timed out waiting for resource to be fully deleted")
			return
		case <-s.C:
			err := c.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, resultObj)
			if err != nil {
				Expect(apierrors.ReasonForError(err)).To(Equal(metav1.StatusReasonNotFound),
					"expected GET to error with NotFound (%s): %s", ref, err)
				return
			}
			s = time.NewTimer(retry)
		}
	}
}

func createUnstructuredAndWait(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)

	err := c.Create(ctx, obj)
	Expect(err).NotTo(HaveOccurred(),
		"expected CREATE to not error (%s): %s", ref, err)

	waitForCreation(ctx, c, obj)
}

func waitForCreation(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	timeout := 30 * time.Second
	retry := 2 * time.Second

	t := time.NewTimer(timeout)
	s := time.NewTimer(0)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			Fail("timed out waiting for resource to be fully created")
			return
		case <-s.C:
			err := c.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, resultObj)
			if err == nil {
				return
			}
			Expect(apierrors.ReasonForError(err)).To(Equal(metav1.StatusReasonNotFound),
				"expected GET to error with NotFound (%s): %s", ref, err)
			// if NotFound, sleep and retry
			s = time.NewTimer(retry)
		}
	}
}

func assertUnstructuredExists(ctx context.Context, c client.Client, obj *unstructured.Unstructured) *unstructured.Unstructured {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	Expect(err).NotTo(HaveOccurred(),
		"expected GET not to error (%s): %s", ref, err)
	return resultObj
}

func assertUnstructuredDoesNotExist(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	Expect(err).To(HaveOccurred(),
		"expected GET to error (%s)", ref)
	Expect(apierrors.ReasonForError(err)).To(Equal(metav1.StatusReasonNotFound),
		"expected GET to error with NotFound (%s): %s", ref, err)
}

func applyUnstructured(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	Expect(err).NotTo(HaveOccurred(),
		"expected GET not to error (%s)", ref)

	err = c.Patch(ctx, obj, client.MergeFrom(resultObj))
	Expect(err).NotTo(HaveOccurred(),
		"expected PATCH not to error (%s): %s", ref, err)
}

func assertUnstructuredAvailable(obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	objc, err := status.GetObjectWithConditions(obj.Object)
	Expect(err).NotTo(HaveOccurred())
	available := false
	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Available": // appsv1.DeploymentAvailable
			if c.Status == "True" { // corev1.ConditionTrue
				available = true
				break
			}
		}
	}
	Expect(available).To(BeTrue(),
		"expected Available condition to be True (%s)", ref)
}

func randomString(prefix string) string {
	randomSuffix := common.RandomStr()
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

func manifestToUnstructured(manifest []byte) *unstructured.Unstructured {
	u := make(map[string]interface{})
	err := yaml.Unmarshal(manifest, &u)
	if err != nil {
		panic(fmt.Errorf("failed to parse manifest yaml: %w", err))
	}
	return &unstructured.Unstructured{
		Object: u,
	}
}

func templateToUnstructured(tmpl string, data interface{}) *unstructured.Unstructured {
	t, err := template.New("manifest").Parse(tmpl)
	if err != nil {
		panic(fmt.Errorf("failed to parse manifest go-template: %w", err))
	}
	var buffer bytes.Buffer
	err = t.Execute(&buffer, data)
	if err != nil {
		panic(fmt.Errorf("failed to execute manifest go-template: %w", err))
	}
	return manifestToUnstructured(buffer.Bytes())
}

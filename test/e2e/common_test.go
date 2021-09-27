// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"strings"
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

func withDependsOn(obj *unstructured.Unstructured, dep string) *unstructured.Unstructured {
	a := obj.GetAnnotations()
	if a == nil {
		a = make(map[string]string, 1)
	}
	a[dependson.Annotation] = dep
	obj.SetAnnotations(a)
	return obj
}

func deleteUnstructuredAndWait(c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.NewResourceReference(obj)

	err := c.Delete(context.TODO(), obj,
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	Expect(err).NotTo(HaveOccurred(),
		"expected DELETE to not error (%s): %s", ref, err)

	waitForDeletion(c, obj)
}

func waitForDeletion(c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.NewResourceReference(obj)
	resultObj := ref.Unstructured()

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
			err := c.Get(context.TODO(), types.NamespacedName{
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

func waitForCreation(c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.NewResourceReference(obj)
	resultObj := ref.Unstructured()

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
			err := c.Get(context.TODO(), types.NamespacedName{
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

func assertUnstructuredExists(c client.Client, obj *unstructured.Unstructured) *unstructured.Unstructured {
	ref := mutation.NewResourceReference(obj)
	resultObj := ref.Unstructured()

	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	Expect(err).NotTo(HaveOccurred(),
		"expected GET not to error (%s): %s", ref, err)
	return resultObj
}

func assertUnstructuredDoesNotExist(c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.NewResourceReference(obj)
	resultObj := ref.Unstructured()

	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	Expect(err).To(HaveOccurred(),
		"expected GET to error (%s)", ref)
	Expect(apierrors.ReasonForError(err)).To(Equal(metav1.StatusReasonNotFound),
		"expected GET to error with NotFound (%s): %s", ref, err)
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
		panic(err)
	}
	return &unstructured.Unstructured{
		Object: u,
	}
}

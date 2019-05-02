/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/client/patch"
)

const serverSideTimeoutSeconds = 10

func deleteDeployment(dep *appsv1.Deployment, ns string) {
	_, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
	if err == nil {
		err = clientset.AppsV1().Deployments(ns).Delete(dep.Name, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteNamespace(ns *corev1.Namespace) {
	_, err := clientset.CoreV1().Namespaces().Get(ns.Name, metav1.GetOptions{})
	if err == nil {
		err = clientset.CoreV1().Namespaces().Delete(ns.Name, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

var _ = Describe("Client", func() {

	var scheme *runtime.Scheme
	var dep *appsv1.Deployment
	var pod *corev1.Pod
	var node *corev1.Node
	var count uint64 = 0
	var replicaCount int32 = 2
	var ns = "default"
	var mergePatch []byte

	BeforeEach(func(done Done) {
		atomic.AddUint64(&count, 1)
		dep = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("deployment-name-%v", count), Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicaCount,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"foo": "bar"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
				},
			},
		}
		// Pod is invalid without a container field in the PodSpec
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%v", count), Namespace: ns},
			Spec:       corev1.PodSpec{},
		}
		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("node-name-%v", count)},
			Spec:       corev1.NodeSpec{},
		}
		scheme = kscheme.Scheme
		var err error
		mergePatch, err = json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"foo": "bar",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		close(done)
	}, serverSideTimeoutSeconds)

	var delOptions *metav1.DeleteOptions
	AfterEach(func(done Done) {
		// Cleanup
		var zero int64 = 0
		policy := metav1.DeletePropagationForeground
		delOptions = &metav1.DeleteOptions{
			GracePeriodSeconds: &zero,
			PropagationPolicy:  &policy,
		}
		deleteDeployment(dep, ns)
		_, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
		if err == nil {
			err = clientset.CoreV1().Nodes().Delete(node.Name, delOptions)
			Expect(err).NotTo(HaveOccurred())
		}
		close(done)
	}, serverSideTimeoutSeconds)

	Describe("New", func() {
		It("should return a new Client", func(done Done) {
			cl, err := client.NewForConfig(dinterface, restmapper)
			Expect(err).NotTo(HaveOccurred())
			Expect(cl).NotTo(BeNil())

			close(done)
		})
	})

	Describe("Create", func() {
		Context("with unstructured objects", func() {
			It("should create a new object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})

				By("creating the object")
				err = cl.Create(context.TODO(), u, nil)
				Expect(err).NotTo(HaveOccurred())

				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				close(done)
			})

			It("should create a new non-namespace object ", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				scheme.Convert(node, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})

				By("creating the object")
				err = cl.Create(context.TODO(), u, nil)
				Expect(err).NotTo(HaveOccurred())

				actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				au := &unstructured.Unstructured{}
				scheme.Convert(actual, au, nil)
				scheme.Convert(node, u, nil)
				By("writing the result back to the go struct")

				Expect(actual.Name).To(Equal(node.Name))

				close(done)
			})

			It("should fail if the object already exists", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				old := dep.DeepCopy()

				By("creating the object")
				cu := &unstructured.Unstructured{}
				scheme.Convert(dep, cu, nil)
				err = cl.Create(context.TODO(), cu, nil)
				Expect(err).NotTo(HaveOccurred())
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				scheme.Convert(old, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})

				By("creating the object a second time")
				err = cl.Create(context.TODO(), u, nil)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsAlreadyExists(err)).To(BeTrue())

				close(done)
			})

			It("should fail if the object does not pass server-side validation", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("creating the pod, since required field Containers is empty")
				u := &unstructured.Unstructured{}
				scheme.Convert(pod, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Version: "v1",
					Kind:    "Pod",
				})
				err = cl.Create(context.TODO(), u, nil)
				Expect(err).To(HaveOccurred())
				close(done)
			}, serverSideTimeoutSeconds)

		})
	})

	Describe("Update", func() {
		Context("with unstructured objects", func() {
			It("should update an existing object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the Deployment")
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				u.SetAnnotations(map[string]string{"foo": "bar"})
				err = cl.Update(context.TODO(), u, nil)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should update an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("updating the object")
				u := &unstructured.Unstructured{}
				scheme.Convert(node, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				u.SetAnnotations(map[string]string{"foo": "bar"})
				err = cl.Update(context.TODO(), u, nil)
				Expect(err).NotTo(HaveOccurred())

				By("validate updated Node had new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})
			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("updating non-existent object")
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Update(context.TODO(), dep, nil)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("Apply", func() {
		Context("with unstructured objects", func() {
			It("should create a new object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the deployment as unstructured")
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				annotation, err := patch.GetModifiedConfiguration(u, true)
				Expect(err).NotTo(HaveOccurred())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})

				By("creating the object")
				err = cl.Apply(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations[v1.LastAppliedConfigAnnotation]).To(Equal(string(annotation)))
				close(done)
			})

			It("should update an existing object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				Expect(err).NotTo(HaveOccurred())
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Apply(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("updating the Deployment")
				u = &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				u.SetAnnotations(map[string]string{"foo": "bar"})
				annotation, err := patch.GetModifiedConfiguration(u, false)
				Expect(err).NotTo(HaveOccurred())
				err = cl.Apply(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				fmt.Printf("c annotation: %s\n", string(annotation))
				fmt.Printf("s annotation: %s\n", actual.Annotations[v1.LastAppliedConfigAnnotation])
				Expect(actual.Annotations[v1.LastAppliedConfigAnnotation]).To(Equal(string(annotation)))
				close(done)
			})

		})
	})

	Describe("StatusClient", func() {
		Context("with unstructured objects", func() {
			It("should update status of an existing object", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the status of Deployment")
				u := &unstructured.Unstructured{}
				dep.Status.Replicas = 1
				scheme.Convert(dep, u, nil)
				err = cl.UpdateStatus(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))

				close(done)
			})

			It("should not update spec of an existing object", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("updating the spec and status of Deployment")
				u := &unstructured.Unstructured{}
				var rc int32 = 1
				dep.Status.Replicas = 1
				dep.Spec.Replicas = &rc
				scheme.Convert(dep, u, nil)
				err = cl.UpdateStatus(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validating updated Deployment has new status and unchanged spec")
				actual, err := clientset.AppsV1().Deployments(ns).Get(dep.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Replicas).To(BeEquivalentTo(1))
				Expect(*actual.Spec.Replicas).To(BeEquivalentTo(replicaCount))

				close(done)
			})

			It("should update an existing object non-namespace object", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("updating status of the object")
				u := &unstructured.Unstructured{}
				node.Status.Phase = corev1.NodeRunning
				scheme.Convert(node, u, nil)
				err = cl.UpdateStatus(context.TODO(), u)
				Expect(err).NotTo(HaveOccurred())

				By("validate updated Node had new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Status.Phase).To(Equal(corev1.NodeRunning))

				close(done)
			})

			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("updating status of a non-existent object")
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				err = cl.UpdateStatus(context.TODO(), u)
				Expect(err).To(HaveOccurred())

				close(done)
			})

			PIt("should fail if the GVK cannot be mapped to a Resource", func() {

			})

			PIt("should fail if an API does not implement Status subresource", func() {

			})

		})
	})

	Describe("Delete", func() {
		Context("with unstructured objects", func() {
			It("should delete an existing object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Deployment")
				depName := dep.Name
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Delete(context.TODO(), u, nil)
				Expect(err).NotTo(HaveOccurred())

				By("validating the Deployment no longer exists")
				_, err = clientset.AppsV1().Deployments(ns).Get(depName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should delete an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Node")
				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("deleting the Node")
				nodeName := node.Name
				u := &unstructured.Unstructured{}
				scheme.Convert(node, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Delete(context.TODO(), u, nil)
				Expect(err).NotTo(HaveOccurred())

				By("validating the Node no longer exists")
				_, err = clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
				Expect(err).To(HaveOccurred())

				close(done)
			})

			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("Deleting node before it is ever created")
				u := &unstructured.Unstructured{}
				scheme.Convert(node, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Delete(context.TODO(), u, nil)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("Patch", func() {
		Context("with unstructured objects", func() {
			It("should patch an existing object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Deployment")
				depName := dep.Name
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), u, patch.Patch{Type: types.MergePatchType, Data: mergePatch}, nil)
				Expect(err).NotTo(HaveOccurred())

				By("validating patched Deployment has new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(depName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should patch an existing object non-namespace object from a go struct", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Node")
				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Node")
				nodeName := node.Name
				u := &unstructured.Unstructured{}
				scheme.Convert(node, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), u, patch.Patch{Type: types.MergePatchType, Data: mergePatch}, nil)
				Expect(err).NotTo(HaveOccurred())

				By("validating pathed Node has new annotation")
				actual, err := clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations["foo"]).To(Equal("bar"))

				close(done)
			})

			It("should fail if the object does not exist", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("Patching node before it is ever created")
				u := &unstructured.Unstructured{}
				scheme.Convert(node, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), u, patch.Patch{Type: types.MergePatchType, Data: mergePatch}, nil)
				Expect(err).To(HaveOccurred())

				close(done)
			})

			PIt("should respect passed-in update options", func() {
				By("creating a new client")
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("initially creating a Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				By("patching the Deployment")
				depName := dep.Name
				u := &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)
				u.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				err = cl.Patch(context.TODO(), u, patch.Patch{Type: types.MergePatchType, Data: mergePatch}, &metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}})
				Expect(err).NotTo(HaveOccurred())

				By("validating patched Deployment does not have the new annotation")
				actual, err := clientset.AppsV1().Deployments(ns).Get(depName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())
				Expect(actual.Annotations).NotTo(HaveKey("foo"))
			})
		})
	})

	Describe("Get", func() {
		Context("with unstructured objects", func() {
			It("should fetch an existing object", func(done Done) {
				By("first creating the Deployment")
				dep, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("encoding the Deployment as unstructured")
				var u runtime.Unstructured = &unstructured.Unstructured{}
				scheme.Convert(dep, u, nil)

				By("fetching the created Deployment")
				var actual unstructured.Unstructured
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "Deployment",
					Version: "v1",
				})
				key := types.NamespacedName{Namespace: ns, Name: dep.Name}
				err = cl.Get(context.TODO(), key, &actual)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())

				By("validating the fetched Deployment equals the created one")
				Expect(u).To(Equal(&actual))

				close(done)
			})

			It("should fetch an existing non-namespace object", func(done Done) {
				By("first creating the Node")
				node, err := clientset.CoreV1().Nodes().Create(node)
				Expect(err).NotTo(HaveOccurred())

				By("encoding the Node as unstructured")
				var u runtime.Unstructured = &unstructured.Unstructured{}
				scheme.Convert(node, u, nil)

				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("fetching the created Node")
				var actual unstructured.Unstructured
				actual.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "",
					Kind:    "Node",
					Version: "v1",
				})
				key := types.NamespacedName{Namespace: ns, Name: node.Name}
				err = cl.Get(context.TODO(), key, &actual)
				Expect(err).NotTo(HaveOccurred())
				Expect(actual).NotTo(BeNil())

				By("validating the fetched Node equals the created one")
				Expect(u).To(Equal(&actual))

				close(done)
			})

			It("should fail if the object does not exists", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())
				Expect(cl).NotTo(BeNil())

				By("fetching object that has not been created yet")
				key := types.NamespacedName{Namespace: ns, Name: dep.Name}
				u := &unstructured.Unstructured{}
				err = cl.Get(context.TODO(), key, u)
				Expect(err).To(HaveOccurred())

				close(done)
			})
		})
	})

	Describe("List", func() {
		Context("with unstructured objects", func() {
			It("should fetch collection of objects", func(done Done) {
				By("create an initial object")
				_, err := clientset.AppsV1().Deployments(ns).Create(dep)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())

				By("listing all objects of that type in the cluster")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				err = cl.List(context.Background(), deps, ns, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(deps.Items).NotTo(BeEmpty())
				hasDep := false
				for _, item := range deps.Items {
					if item.GetName() == dep.Name && item.GetNamespace() == dep.Namespace {
						hasDep = true
						break
					}
				}
				Expect(hasDep).To(BeTrue())
				close(done)
			}, serverSideTimeoutSeconds)

			It("should return an empty list if there are no matching objects", func(done Done) {
				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in the cluster")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				Expect(cl.List(context.Background(), deps, ns, nil)).NotTo(HaveOccurred())

				By("validating no Deployments are returned")
				Expect(deps.Items).To(BeEmpty())

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector", func(done Done) {
				By("creating a Deployment in test-namespace-5")
				tns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-5"}}
				_, err := clientset.CoreV1().Namespaces().Create(tns1)
				Expect(err).NotTo(HaveOccurred())
				depFrontend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "deployment-frontend", Namespace: "test-namespace-5"},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "frontend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
						},
					},
				}
				depFrontend, err = clientset.AppsV1().Deployments("test-namespace-5").Create(depFrontend)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-6")
				tns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-6"}}
				_, err = clientset.CoreV1().Namespaces().Create(tns2)
				Expect(err).NotTo(HaveOccurred())
				depBackend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "deployment-backend", Namespace: "test-namespace-6"},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "backend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "backend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
						},
					},
				}
				depBackend, err = clientset.AppsV1().Deployments("test-namespace-6").Create(depBackend)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in test-namespace-5")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				err = cl.List(context.Background(), deps, "test-namespace-5", nil)
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment in test-namespace-5 is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.GetName()).To(Equal("deployment-frontend"))

				deleteDeployment(depFrontend, "test-namespace-5")
				deleteDeployment(depBackend, "test-namespace-6")
				deleteNamespace(tns1)
				deleteNamespace(tns2)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by field selector", func(done Done) {
				By("creating a Deployment with name deployment-frontend")
				depFrontend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "deployment-frontend", Namespace: ns},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "frontend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
						},
					},
				}
				depFrontend, err := clientset.AppsV1().Deployments(ns).Create(depFrontend)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment with name deployment-backend")
				depBackend := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "deployment-backend", Namespace: ns},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "backend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "backend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
						},
					},
				}
				depBackend, err = clientset.AppsV1().Deployments(ns).Create(depBackend)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments with field metadata.name=deployment-backend")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				err = cl.List(context.Background(), deps, "",
					&metav1.ListOptions{
						FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": "deployment-backend"}).String(),
					})
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment with the backend field is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.GetName()).To(Equal("deployment-backend"))

				deleteDeployment(depFrontend, ns)
				deleteDeployment(depBackend, ns)

				close(done)
			}, serverSideTimeoutSeconds)

			It("should filter results by namespace selector and label selector", func(done Done) {
				By("creating a Deployment in test-namespace-7 with the app=frontend label")
				tns3 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-7"}}
				_, err := clientset.CoreV1().Namespaces().Create(tns3)
				Expect(err).NotTo(HaveOccurred())
				depFrontend3 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-frontend",
						Namespace: "test-namespace-7",
						Labels:    map[string]string{"app": "frontend"},
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "frontend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
						},
					},
				}
				depFrontend3, err = clientset.AppsV1().Deployments("test-namespace-7").Create(depFrontend3)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-7 with the app=backend label")
				depBackend3 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-backend",
						Namespace: "test-namespace-7",
						Labels:    map[string]string{"app": "backend"},
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "backend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "backend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
						},
					},
				}
				depBackend3, err = clientset.AppsV1().Deployments("test-namespace-7").Create(depBackend3)
				Expect(err).NotTo(HaveOccurred())

				By("creating a Deployment in test-namespace-8 with the app=frontend label")
				tns4 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace-8"}}
				_, err = clientset.CoreV1().Namespaces().Create(tns4)
				Expect(err).NotTo(HaveOccurred())
				depFrontend4 := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-frontend",
						Namespace: "test-namespace-8",
						Labels:    map[string]string{"app": "frontend"},
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "frontend"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "frontend"}},
							Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
						},
					},
				}
				depFrontend4, err = clientset.AppsV1().Deployments("test-namespace-8").Create(depFrontend4)
				Expect(err).NotTo(HaveOccurred())

				cl, err := client.NewForConfig(dinterface, restmapper)
				Expect(err).NotTo(HaveOccurred())

				By("listing all Deployments in test-namespace-8 with label app=frontend")
				deps := &unstructured.UnstructuredList{}
				deps.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   "apps",
					Kind:    "DeploymentList",
					Version: "v1",
				})
				lbls := map[string]string{"app": "frontend"}
				err = cl.List(context.Background(), deps, "test-namespace-7",
					&metav1.ListOptions{
						LabelSelector: labels.SelectorFromSet(lbls).String(),
					})
				Expect(err).NotTo(HaveOccurred())

				By("only the Deployment in test-namespace-7 with label app=frontend is returned")
				Expect(deps.Items).NotTo(BeEmpty())
				Expect(1).To(Equal(len(deps.Items)))
				actual := deps.Items[0]
				Expect(actual.GetName()).To(Equal("deployment-frontend"))
				Expect(actual.GetNamespace()).To(Equal("test-namespace-7"))

				deleteDeployment(depFrontend3, "test-namespace-7")
				deleteDeployment(depBackend3, "test-namespace-7")
				deleteDeployment(depFrontend4, "test-namespace-8")
				deleteNamespace(tns3)
				deleteNamespace(tns4)

				close(done)
			}, serverSideTimeoutSeconds)

			PIt("should fail if the object doesn't have meta", func() {

			})
		})
	})
})

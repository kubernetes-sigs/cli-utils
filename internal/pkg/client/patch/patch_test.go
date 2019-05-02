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

package patch_test

import (
	//"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/cli-experimental/internal/pkg/client/patch"
)

var _ = Describe("Patch", func() {

	var scheme *runtime.Scheme
	var dep *appsv1.Deployment
	var unstructuredDeployment, modifiedDeployment *unstructured.Unstructured
	var unstructuredCRD, modifiedCRD *unstructured.Unstructured
	var replicaCount int32 = 2
	var ns = "default"
	var count uint64 = 0
	var annotatedSerializedDeployment string
	var serializedDeployment string
	var mergePatch string

	BeforeEach(func(done Done) {
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

		annotatedSerializedDeployment = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"name\":\"deployment-name-0\",\"namespace\":\"default\"},\"spec\":{\"replicas\":2,\"selector\":{\"matchLabels\":{\"foo\":\"bar\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"foo\":\"bar\"}},\"spec\":{\"containers\":[{\"image\":\"nginx\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"},"creationTimestamp":null,"name":"deployment-name-0","namespace":"default"},"spec":{"replicas":2,"selector":{"matchLabels":{"foo":"bar"}},"strategy":{},"template":{"metadata":{"creationTimestamp":null,"labels":{"foo":"bar"}},"spec":{"containers":[{"image":"nginx","name":"nginx","resources":{}}]}}},"status":{}}
`
		serializedDeployment = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"annotations":{},"creationTimestamp":null,"name":"deployment-name-0","namespace":"default"},"spec":{"replicas":2,"selector":{"matchLabels":{"foo":"bar"}},"strategy":{},"template":{"metadata":{"creationTimestamp":null,"labels":{"foo":"bar"}},"spec":{"containers":[{"image":"nginx","name":"nginx","resources":{}}]}}},"status":{}}
`
		mergePatch = `{"spec":{"template":{"spec":{"containers":[{"image":"nginx23","name":"nginx","resources":{}}]}}}}`
		unstructuredDeployment = &unstructured.Unstructured{}
		modifiedDeployment = &unstructured.Unstructured{}
		unstructuredCRD = &unstructured.Unstructured{}
		modifiedCRD = &unstructured.Unstructured{}
		scheme = kscheme.Scheme
		scheme.Convert(dep, unstructuredDeployment, nil)
		scheme.Convert(dep, unstructuredCRD, nil)
		dep.Spec.Template.Spec.Containers[0].Image = "nginx23"
		scheme.Convert(dep, modifiedDeployment, nil)
		scheme.Convert(dep, modifiedCRD, nil)
		modifiedDeployment.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "apps",
			Kind:    "Deployment",
			Version: "v1",
		})
		unstructuredDeployment.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "apps",
			Kind:    "Deployment",
			Version: "v1",
		})
		modifiedCRD.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "mydomain",
			Kind:    "Something",
			Version: "v1",
		})
		unstructuredCRD.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "mydomain",
			Kind:    "Something",
			Version: "v1",
		})
		close(done)
	})

	AfterEach(func(done Done) {
		// Cleanup
		close(done)
	})

	Describe("Patch", func() {
		Context("SerializeLastApplied", func() {
			It("should get correct serialization of incoming object", func(done Done) {
				By("getting the modified config for deployment")
				modifiedconfig, err := patch.SerializeLastApplied(unstructuredDeployment, true)
				Expect(err).NotTo(HaveOccurred())

				By("checking expected modified config")
				Expect(string(modifiedconfig)).To(Equal(annotatedSerializedDeployment))
				close(done)
			})

			It("should get correct non-annotated serialization of incoming object", func(done Done) {
				By("getting the modified config for deployment")
				modifiedconfig, err := patch.SerializeLastApplied(unstructuredDeployment, false)
				Expect(err).NotTo(HaveOccurred())

				By("checking expected modified config")
				Expect(string(modifiedconfig)).To(Equal(serializedDeployment))
				close(done)
			})

		})

		Context("SetLastApplied", func() {
			It("should annotate the lastapplied of incoming object", func(done Done) {
				By("set last applied")
				err := patch.SetLastApplied(unstructuredDeployment)
				Expect(err).NotTo(HaveOccurred())

				By("checking expected annotation")
				lastapplied, err := patch.GetLastApplied(unstructuredDeployment)
				Expect(err).NotTo(HaveOccurred())
				Expect((string(lastapplied))).To(Equal(serializedDeployment))
				close(done)
			})

		})

		Context("GetMergePatch", func() {
			It("should return the correct patch forincoming object", func(done Done) {
				By("get merge patch")
				p, err := patch.GetMergePatch(unstructuredDeployment, modifiedDeployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(p.Type).To(Equal(types.MergePatchType))

				By("checking expected patch")
				Expect((string(p.Data))).To(Equal(mergePatch))
				close(done)
			})

		})

		Context("GetClientSideApplyPatch", func() {
			It("should return correct last-applied patch for objects without last-applied annotation", func(done Done) {
				By("get merge patch without last-applied annotation and using same object")
				p, err := patch.GetClientSideApplyPatch(unstructuredDeployment, unstructuredDeployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(p.Type).To(Equal(types.StrategicMergePatchType))

				By("checking expected patch to have only metadata.annotation")
				Expect((string(p.Data))).To(Equal(`{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"name\":\"deployment-name-0\",\"namespace\":\"default\"},\"spec\":{\"replicas\":2,\"selector\":{\"matchLabels\":{\"foo\":\"bar\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"foo\":\"bar\"}},\"spec\":{\"containers\":[{\"image\":\"nginx\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"}}}`))

				By("get patch by changing deployment pod image")
				p, err = patch.GetClientSideApplyPatch(unstructuredDeployment, modifiedDeployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(p.Type).To(Equal(types.StrategicMergePatchType))

				By("checking expected patch to have metadata.annotation amd spec.template.spec changes in patch")
				Expect((string(p.Data))).To(Equal(`{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"name\":\"deployment-name-0\",\"namespace\":\"default\"},\"spec\":{\"replicas\":2,\"selector\":{\"matchLabels\":{\"foo\":\"bar\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"foo\":\"bar\"}},\"spec\":{\"containers\":[{\"image\":\"nginx23\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"}},"spec":{"template":{"spec":{"$setElementOrder/containers":[{"name":"nginx"}],"containers":[{"image":"nginx23","name":"nginx"}]}}}}`))
				close(done)
			})

			It("should return correct last-applied patch for objects with last-applied annotation", func(done Done) {
				By("set last applied annotation")
				err := patch.SetLastApplied(unstructuredDeployment)
				Expect(err).NotTo(HaveOccurred())

				By("get merge patch with last-applied annotation and no change")
				p, err := patch.GetClientSideApplyPatch(unstructuredDeployment, unstructuredDeployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(p.Type).To(Equal(types.StrategicMergePatchType))

				By("checking expected patch to be {}")
				Expect((string(p.Data))).To(Equal(`{}`))

				By("get merge patch with last-applied annotation and change in deployment image")
				p, err = patch.GetClientSideApplyPatch(unstructuredDeployment, modifiedDeployment)
				Expect(err).NotTo(HaveOccurred())
				Expect(p.Type).To(Equal(types.StrategicMergePatchType))

				By("checking expected patch to have metadata.annotation amd spec.template.spec changes in patch")
				Expect((string(p.Data))).To(Equal(`{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"name\":\"deployment-name-0\",\"namespace\":\"default\"},\"spec\":{\"replicas\":2,\"selector\":{\"matchLabels\":{\"foo\":\"bar\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"foo\":\"bar\"}},\"spec\":{\"containers\":[{\"image\":\"nginx23\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"}},"spec":{"template":{"spec":{"$setElementOrder/containers":[{"name":"nginx"}],"containers":[{"image":"nginx23","name":"nginx"}]}}}}`))
				close(done)
			})

			It("should return correct last-applied patch for unregistered objects", func(done Done) {
				By("get merge patch with last-applied annotation and no change")
				p, err := patch.GetClientSideApplyPatch(unstructuredCRD, unstructuredCRD)
				Expect(err).NotTo(HaveOccurred())
				Expect(p.Type).To(Equal(types.MergePatchType))

				By("checking expected patch to have metadata.annotation and .spec.template.metadata.creationTimestamp ")
				Expect((string(p.Data))).To(Equal(`{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"mydomain/v1\",\"kind\":\"Something\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"name\":\"deployment-name-0\",\"namespace\":\"default\"},\"spec\":{\"replicas\":2,\"selector\":{\"matchLabels\":{\"foo\":\"bar\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"foo\":\"bar\"}},\"spec\":{\"containers\":[{\"image\":\"nginx\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"},"creationTimestamp":null},"spec":{"template":{"metadata":{"creationTimestamp":null}}}}`))

				By("get merge patch with last-applied annotation and change in deployment image")
				p, err = patch.GetClientSideApplyPatch(unstructuredCRD, modifiedCRD)
				Expect(err).NotTo(HaveOccurred())
				Expect(p.Type).To(Equal(types.MergePatchType))

				By("checking expected patch to have metadata.annotation amd spec.template.[metadata,spec.containers]")
				Expect((string(p.Data))).To(Equal(`{"metadata":{"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"apiVersion\":\"mydomain/v1\",\"kind\":\"Something\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"name\":\"deployment-name-0\",\"namespace\":\"default\"},\"spec\":{\"replicas\":2,\"selector\":{\"matchLabels\":{\"foo\":\"bar\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"foo\":\"bar\"}},\"spec\":{\"containers\":[{\"image\":\"nginx23\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"},"creationTimestamp":null},"spec":{"template":{"metadata":{"creationTimestamp":null},"spec":{"containers":[{"image":"nginx23","name":"nginx","resources":{}}]}}}}`))
				close(done)
			})

		})
	})
})

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package e2eutil

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-utils/pkg/apply/event"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/flowcontrol"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object/dependson"
	"sigs.k8s.io/cli-utils/pkg/object/mutation"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/cli-utils/test/e2e/customprovider"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const TestIDLabel = "test-id"

func WithReplicas(obj *unstructured.Unstructured, replicas int) *unstructured.Unstructured {
	err := unstructured.SetNestedField(obj.Object, int64(replicas), "spec", "replicas")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return obj
}

func WithNamespace(obj *unstructured.Unstructured, namespace string) *unstructured.Unstructured {
	obj.SetNamespace(namespace)
	return obj
}

func PodWithImage(obj *unstructured.Unstructured, containerName, image string) *unstructured.Unstructured {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "containers")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(found).To(gomega.BeTrue())

	containerFound := false
	for i := range containers {
		container := containers[i].(map[string]any)
		name := container["name"].(string)
		if name != containerName {
			continue
		}
		containerFound = true
		container["image"] = image
	}
	gomega.Expect(containerFound).To(gomega.BeTrue())
	err = unstructured.SetNestedSlice(obj.Object, containers, "spec", "containers")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return obj
}

func WithNodeSelector(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	selectors, found, err := unstructured.NestedMap(obj.Object, "spec", "nodeSelector")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if !found {
		selectors = make(map[string]any)
	}
	selectors[key] = value
	err = unstructured.SetNestedMap(obj.Object, selectors, "spec", "nodeSelector")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return obj
}

func WithAnnotation(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
	return obj
}

func WithDependsOn(obj *unstructured.Unstructured, dep string) *unstructured.Unstructured {
	a := obj.GetAnnotations()
	if a == nil {
		a = make(map[string]string, 1)
	}
	a[dependson.Annotation] = dep
	obj.SetAnnotations(a)
	return obj
}

func WithFinalizer(obj *unstructured.Unstructured, finalizer string) *unstructured.Unstructured {
	finalizers := obj.GetFinalizers()
	finalizers = append(finalizers, finalizer)
	obj.SetFinalizers(finalizers)
	return obj
}

func WithoutFinalizers(obj *unstructured.Unstructured) *unstructured.Unstructured {
	obj.SetFinalizers([]string{})
	return obj
}

func DeleteUnstructuredAndWait(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)

	err := c.Delete(ctx, obj,
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(),
		"expected DELETE to not error (%s): %s", ref, err)

	WaitForDeletion(ctx, c, obj)
}

func WaitForDeletion(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
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
			ginkgo.Fail("timed out waiting for resource to be fully deleted")
			return
		case <-s.C:
			err := c.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, resultObj)
			if err != nil {
				gomega.Expect(apierrors.ReasonForError(err)).To(gomega.Equal(metav1.StatusReasonNotFound),
					"expected GET to error with NotFound (%s): %s", ref, err)
				return
			}
			s = time.NewTimer(retry)
		}
	}
}

func CreateUnstructuredAndWait(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)

	err := c.Create(ctx, obj)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(),
		"expected CREATE to not error (%s): %s", ref, err)

	WaitForCreation(ctx, c, obj)
}

func WaitForCreation(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
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
			ginkgo.Fail("timed out waiting for resource to be fully created")
			return
		case <-s.C:
			err := c.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, resultObj)
			if err == nil {
				return
			}
			gomega.Expect(apierrors.ReasonForError(err)).To(gomega.Equal(metav1.StatusReasonNotFound),
				"expected GET to error with NotFound (%s): %s", ref, err)
			// if NotFound, sleep and retry
			s = time.NewTimer(retry)
		}
	}
}

func AssertUnstructuredExists(ctx context.Context, c client.Client, obj *unstructured.Unstructured) *unstructured.Unstructured {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(),
		"expected GET not to error (%s): %s", ref, err)
	return resultObj
}

func AssertHasDeletionTimestamp(ctx context.Context, c client.Client, obj *unstructured.Unstructured) *unstructured.Unstructured {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(),
		"expected GET not to error (%s): %s", ref, err)
	gomega.Expect(resultObj.GetDeletionTimestamp().IsZero()).To(gomega.BeFalse(),
		"expected deletion timestamp to be non-zero (%s)", ref)
	return resultObj
}

func AssertUnstructuredDoesNotExist(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	gomega.Expect(err).To(gomega.HaveOccurred(),
		"expected GET to error (%s)", ref)
	gomega.Expect(apierrors.ReasonForError(err)).To(gomega.Equal(metav1.StatusReasonNotFound),
		"expected GET to error with NotFound (%s): %s", ref, err)
}

func ApplyUnstructured(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	resultObj := ref.ToUnstructured()

	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, resultObj)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(),
		"expected GET not to error (%s)", ref)

	err = c.Patch(ctx, obj, client.MergeFrom(resultObj))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(),
		"expected PATCH not to error (%s): %s", ref, err)
}

func AssertUnstructuredAvailable(obj *unstructured.Unstructured) {
	ref := mutation.ResourceReferenceFromUnstructured(obj)
	objc, err := status.GetObjectWithConditions(obj.Object)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	available := false
	for _, c := range objc.Status.Conditions {
		// appsv1.DeploymentAvailable && corev1.ConditionTrue
		if c.Type == "Available" && c.Status == "True" {
			available = true
			break
		}
	}
	gomega.Expect(available).To(gomega.BeTrue(),
		"expected Available condition to be True (%s)", ref)
}

func AssertUnstructuredCount(ctx context.Context, c client.Client, obj *unstructured.Unstructured, count int) {
	var u unstructured.UnstructuredList
	u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	err := c.List(ctx, &u,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels(obj.GetLabels()))
	if err != nil && count == 0 {
		expectNotFoundError(err)
		return
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(len(u.Items)).To(gomega.Equal(count), "unexpected number of %s", obj.GetKind())
}

func RandomString(prefix string) string {
	randomSuffix := common.RandomStr()
	return fmt.Sprintf("%s%s", prefix, randomSuffix)
}

func Run(ch <-chan event.Event) error {
	var err error
	for e := range ch {
		if e.Type == event.ErrorType {
			err = e.ErrorEvent.Err
		}
	}
	return err
}

var RunWithNoErr = RunCollectNoErr

func RunCollect(ch <-chan event.Event) []event.Event {
	var events []event.Event
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func RunCollectNoErr(ch <-chan event.Event, callerSkip ...int) []event.Event {
	skip := 0
	if len(callerSkip) > 0 {
		skip = callerSkip[0]
	}

	events := RunCollect(ch)
	ExpectNoEventErrors(events, skip+1)
	ExpectNoReconcileTimeouts(events, skip+1)
	return events
}

func ExpectNoEventErrors(events []event.Event, callerSkip ...int) {
	skip := 0
	if len(callerSkip) > 0 {
		skip = callerSkip[0]
	}

	gomega.Expect(events).WithOffset(skip + 1).NotTo(
		gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
			gstruct.Fields{
				"Type": gomega.Equal(event.ErrorType),
			})))
	gomega.Expect(events).WithOffset(skip + 1).NotTo(
		gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
			gstruct.Fields{
				"Type": gomega.Equal(event.ApplyType),
				"ApplyEvent": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Status": gomega.Equal(event.ApplyFailed),
				}),
			})))
	gomega.Expect(events).WithOffset(skip + 1).NotTo(
		gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
			gstruct.Fields{
				"Type": gomega.Equal(event.PruneType),
				"PruneEvent": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Status": gomega.Equal(event.PruneFailed),
				}),
			})))
	gomega.Expect(events).WithOffset(skip + 1).NotTo(
		gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
			gstruct.Fields{
				"Type": gomega.Equal(event.DeleteType),
				"DeleteEvent": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Status": gomega.Equal(event.DeleteFailed),
				}),
			})))
}

func ExpectNoReconcileTimeouts(events []event.Event, callerSkip ...int) {
	skip := 0
	if len(callerSkip) > 0 {
		skip = callerSkip[0]
	}

	gomega.Expect(events).WithOffset(skip + 1).NotTo(
		gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
			gstruct.Fields{
				"Type": gomega.Equal(event.WaitType),
				"WaitEvent": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Status": gomega.Equal(event.ReconcileTimeout),
				}),
			})))
}

func ManifestToUnstructured(manifest []byte) *unstructured.Unstructured {
	u := make(map[string]any)
	err := yaml.Unmarshal(manifest, &u)
	if err != nil {
		panic(fmt.Errorf("failed to parse manifest yaml: %w", err))
	}
	return &unstructured.Unstructured{
		Object: u,
	}
}

func TemplateToUnstructured(tmpl string, data any) *unstructured.Unstructured {
	t, err := template.New("manifest").Parse(tmpl)
	if err != nil {
		panic(fmt.Errorf("failed to parse manifest go-template: %w", err))
	}
	var buffer bytes.Buffer
	err = t.Execute(&buffer, data)
	if err != nil {
		panic(fmt.Errorf("failed to execute manifest go-template: %w", err))
	}
	return ManifestToUnstructured(buffer.Bytes())
}

func CreateInventoryCRD(ctx context.Context, c client.Client) {
	invCRD := ManifestToUnstructured(customprovider.InventoryCRD)
	var u unstructured.Unstructured
	u.SetGroupVersionKind(invCRD.GroupVersionKind())
	err := c.Get(ctx, types.NamespacedName{
		Name: invCRD.GetName(),
	}, &u)
	if apierrors.IsNotFound(err) {
		err = c.Create(ctx, invCRD)
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	gomega.Eventually(func() bool { // wait for CRD to be processed by the API server.
		err = c.Get(ctx, types.NamespacedName{
			Name: invCRD.GetName(),
		}, &u)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		res, err := status.CRDConditions(&u)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		return res.Status == status.CurrentStatus
	}).WithContext(ctx).WithTimeout(time.Minute).Should(gomega.BeTrue())
}

func CreateRandomNamespace(ctx context.Context, c client.Client) *v1.Namespace {
	namespaceName := RandomString("e2e-test-")
	namespace := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	err := c.Create(ctx, namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	return namespace
}

func DeleteInventoryCRD(ctx context.Context, c client.Client) {
	invCRD := ManifestToUnstructured(customprovider.InventoryCRD)
	DeleteUnstructuredIfExists(ctx, c, invCRD)
}

func DeleteUnstructuredIfExists(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	err := c.Delete(ctx, obj)
	if err != nil {
		expectNotFoundError(err)
	}
}

func DeleteAllUnstructuredIfExists(ctx context.Context, c client.Client, obj *unstructured.Unstructured) {
	err := c.DeleteAllOf(ctx, obj,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels(obj.GetLabels()))
	if err != nil {
		expectNotFoundError(err)
	}
}

func DeleteNamespace(ctx context.Context, c client.Client, namespace *v1.Namespace) {
	err := c.Delete(ctx, namespace)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

func UnstructuredExistsAndIsNotTerminating(ctx context.Context, c client.Client, obj *unstructured.Unstructured) bool {
	serverObj := obj.DeepCopy()
	err := c.Get(ctx, types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, serverObj)
	if err != nil {
		expectNotFoundError(err)
		return false
	}
	return !UnstructuredIsTerminating(serverObj)
}

func expectNotFoundError(err error) {
	gomega.Expect(err).To(gomega.Or(
		gomega.BeAssignableToTypeOf(&meta.NoKindMatchError{}),
		gomega.BeAssignableToTypeOf(&apierrors.StatusError{}),
	))
	if se, ok := err.(*apierrors.StatusError); ok {
		gomega.Expect(se.ErrStatus.Reason).To(gomega.Or(
			gomega.Equal(metav1.StatusReasonNotFound),
			// custom resources dissalow deletion if the CRD is terminating
			gomega.Equal(metav1.StatusReasonMethodNotAllowed),
		))
	}
}

func UnstructuredIsTerminating(obj *unstructured.Unstructured) bool {
	objc, err := status.GetObjectWithConditions(obj.Object)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	for _, c := range objc.Status.Conditions {
		if c.Type == "Terminating" && c.Status == "True" {
			return true
		}
	}
	return false
}

func UnstructuredNamespace(name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("v1")
	u.SetKind("Namespace")
	u.SetName(name)
	return u
}

func IsFlowControlEnabled(config *rest.Config) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	enabled, err := flowcontrol.IsEnabled(ctx, config)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return enabled
}

// FilterOptionalEvents looks for optional events in the expected list and
// removes them from both lists. This allows the output to be compared for
// equality.
//
// Optional events include:
// - WaitEvent with ReconcilePending
func FilterOptionalEvents(expected, received []testutil.ExpEvent) ([]testutil.ExpEvent, []testutil.ExpEvent) {
	expectedCopy := make([]testutil.ExpEvent, 0, len(expected))
	for _, ee := range expected {
		if ee.EventType == event.WaitType &&
			ee.WaitEvent != nil &&
			ee.WaitEvent.Status == event.ReconcilePending {
			// Pending WaitEvent is optional.
			// Remove first event match, if exists.
			for i, re := range received {
				if cmp.Equal(re, ee, cmpopts.EquateErrors()) {
					// remove event at index i
					received = append(received[:i], received[i+1:]...)
					break
				}
			}
		} else {
			expectedCopy = append(expectedCopy, ee)
		}
	}
	return expectedCopy, received
}

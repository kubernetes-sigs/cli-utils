// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/apis/testapigroup"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/test"
	"k8s.io/apimachinery/pkg/watch"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cli-utils/pkg/testutil"
)

func TestResourceNotFoundError(t *testing.T) {
	carpGVK := schema.GroupVersionKind{
		Group:   "foo",
		Version: "v1",
		Kind:    "Carp",
	}
	exampleGR := schema.GroupResource{
		Group:    carpGVK.Group,
		Resource: "carps",
	}
	namespace := "example-ns"

	testCases := []struct {
		name         string
		setup        func(*dynamicfake.FakeDynamicClient)
		errorHandler func(t *testing.T, err error)
	}{
		{
			name: "List resource not found error",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				fakeClient.PrependReactor("list", exampleGR.Resource, func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					listAction := action.(clienttesting.ListAction)
					if listAction.GetNamespace() != namespace {
						assert.Fail(t, "Received unexpected LIST namespace: %s", listAction.GetNamespace())
						return false, nil, nil
					}
					// dynamicClient converts Status objects from the apiserver into errors.
					// So we can just return the right error here to simulate an error from
					// the apiserver.
					err = newGenericServerResponse(action, newNotFoundResourceStatusError(action))
					return true, nil, err
				})
			},
			errorHandler: func(t *testing.T, err error) {
				switch {
				case apierrors.IsNotFound(err):
					t.Logf("Received expected typed NotFound error: %v", err)
				default:
					// If we got this error, the test is probably broken.
					t.Errorf("Expected typed NotFound error, but got a different error: %v", err)
				}
			},
		},
		{
			name: "Watch resource not found error",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				fakeClient.PrependWatchReactor(exampleGR.Resource, func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
					// dynamicClient converts Status objects from the apiserver into errors.
					// So we can just return the right error here to simulate an error from
					// the apiserver.
					err = newGenericServerResponse(action, newNotFoundResourceStatusError(action))
					return true, nil, err
				})
			},
			errorHandler: func(t *testing.T, err error) {
				switch {
				case apierrors.IsNotFound(err):
					// This is the expected behavior, because the
					// Informer/Reflector DOES wrap watch errors
					t.Logf("Received expected typed NotFound error: %v", err)
				default:
					// If we got this error, the test is probably broken.
					t.Errorf("Expected typed NotFound error, but got a different error: %v", err)
				}
			},
		},
		{
			name: "List resource forbidden error",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				fakeClient.PrependReactor("list", exampleGR.Resource, func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					listAction := action.(clienttesting.ListAction)
					if listAction.GetNamespace() != namespace {
						assert.Fail(t, "Received unexpected LIST namespace: %s", listAction.GetNamespace())
						return false, nil, nil
					}
					// dynamicClient converts Status objects from the apiserver into errors.
					// So we can just return the right error here to simulate an error from
					// the apiserver.
					err = newGenericServerResponse(action, newForbiddenResourceStatusError(action))
					return true, nil, err
				})
			},
			errorHandler: func(t *testing.T, err error) {
				switch {
				case apierrors.IsForbidden(err):
					t.Logf("Received expected typed Forbidden error: %v", err)
				default:
					// If we got this error, the test is probably broken.
					t.Errorf("Expected typed Forbidden error, but got a different error: %v", err)
				}
			},
		},
		{
			name: "Watch resource forbidden error",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				fakeClient.PrependWatchReactor(exampleGR.Resource, func(action clienttesting.Action) (handled bool, ret watch.Interface, err error) {
					// dynamicClient converts Status objects from the apiserver into errors.
					// So we can just return the right error here to simulate an error from
					// the apiserver.
					err = newGenericServerResponse(action, newForbiddenResourceStatusError(action))
					return true, nil, err
				})
			},
			errorHandler: func(t *testing.T, err error) {
				switch {
				case apierrors.IsForbidden(err):
					// This is the expected behavior, because the
					// Informer/Reflector DOES wrap watch errors
					t.Logf("Received expected typed Forbidden error: %v", err)
				default:
					// If we got this error, the test is probably broken.
					t.Errorf("Expected typed Forbidden error, but got a different error: %v", err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypes(metav1.SchemeGroupVersion, &metav1.Status{})

			// Register foo/v1 Carp CRD
			scheme.AddKnownTypes(carpGVK.GroupVersion(), &testapigroup.Carp{}, &testapigroup.CarpList{}, &test.List{})

			// Fake client that only knows about the types registered to the scheme
			fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

			// log fakeClient calls
			fakeClient.PrependReactor("*", "*", func(a clienttesting.Action) (bool, runtime.Object, error) {
				klog.V(3).Infof("FakeDynamicClient: %T{ Verb: %q, Resource: %q, Namespace: %q }",
					a, a.GetVerb(), a.GetResource().Resource, a.GetNamespace())
				return false, nil, nil
			})
			fakeClient.PrependWatchReactor("*", func(a clienttesting.Action) (bool, watch.Interface, error) {
				klog.V(3).Infof("FakeDynamicClient: %T{ Verb: %q, Resource: %q, Namespace: %q }",
					a, a.GetVerb(), a.GetResource().Resource, a.GetNamespace())
				return false, nil, nil
			})

			tc.setup(fakeClient)

			informerFactory := NewDynamicInformerFactory(fakeClient, 0) // disable re-sync

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			fakeMapper := testutil.NewFakeRESTMapper(carpGVK)
			mapping, err := fakeMapper.RESTMapping(carpGVK.GroupKind())
			require.NoError(t, err)

			informer := informerFactory.NewInformer(ctx, mapping, namespace)

			err = informer.SetWatchErrorHandler(func(_ *cache.Reflector, err error) {
				tc.errorHandler(t, err)
				// Stop the informer after the first error.
				cancel()
			})
			require.NoError(t, err)

			// Block until context cancel or timeout.
			informer.Run(ctx.Done())
		})
	}
}

// newForbiddenResourceStatusError emulates a Forbidden error from the apiserver
// for a namespace-scoped resource.
// https://github.com/kubernetes/apiserver/blob/master/pkg/endpoints/handlers/responsewriters/errors.go#L36
func newForbiddenResourceStatusError(action clienttesting.Action) *apierrors.StatusError {
	username := "unused"
	verb := action.GetVerb()
	resource := action.GetResource().Resource
	if subresource := action.GetSubresource(); len(subresource) > 0 {
		resource = resource + "/" + subresource
	}
	apiGroup := action.GetResource().Group
	namespace := action.GetNamespace()

	// https://github.com/kubernetes/apiserver/blob/master/pkg/endpoints/handlers/responsewriters/errors.go#L51
	err := fmt.Errorf("User %q cannot %s resource %q in API group %q in the namespace %q",
		username, verb, resource, apiGroup, namespace)

	qualifiedResource := action.GetResource().GroupResource()
	name := "" // unused by ListAndWatch
	return apierrors.NewForbidden(qualifiedResource, name, err)
}

// newNotFoundResourceStatusError emulates a NotFOund error from the apiserver
// for a resource (not an object).
func newNotFoundResourceStatusError(action clienttesting.Action) *apierrors.StatusError {
	qualifiedResource := action.GetResource().GroupResource()
	name := "" // unused by ListAndWatch
	return apierrors.NewNotFound(qualifiedResource, name)
}

// newGenericServerResponse emulates a StatusError from the apiserver.
func newGenericServerResponse(action clienttesting.Action, statusError *apierrors.StatusError) *apierrors.StatusError {
	errorCode := int(statusError.ErrStatus.Code)
	verb := action.GetVerb()
	qualifiedResource := action.GetResource().GroupResource()
	name := statusError.ErrStatus.Details.Name
	// https://github.com/kubernetes/apimachinery/blob/v0.24.0/pkg/api/errors/errors.go#L435
	return apierrors.NewGenericServerResponse(errorCode, verb, qualifiedResource, name, statusError.Error(), -1, false)
}

func TestNewFilteredListWatchFromDynamicClientList(t *testing.T) {
	fakeMapper := testutil.NewFakeRESTMapper(
		v1.SchemeGroupVersion.WithKind("Pod"),
	)

	pod1 := yamlToUnstructured(t, pod1Yaml)
	pod1.SetNamespace("ns-1")
	// pod1ID := object.UnstructuredToObjMetadata(pod1)
	// pod1Current := yamlToUnstructured(t, pod1CurrentYaml)
	podGVR := getGVR(t, fakeMapper, pod1)

	pod2 := pod1.DeepCopy()
	pod2.SetNamespace("ns-2")
	pod2.SetName("pod-2")

	labelKey := "example-key-2"
	labelValue := "example-value-2"
	pod2Labels := pod2.GetLabels()
	if pod2Labels == nil {
		pod2Labels = map[string]string{}
	}
	pod2Labels[labelKey] = labelValue
	pod2.SetLabels(pod2Labels)

	pod3 := pod1.DeepCopy()
	pod3.SetNamespace("ns-2")
	pod3.SetName("pod-3")

	annotationKey := "example-key-3"
	annotationValue := "example-value-3"
	pod3Annotations := pod3.GetAnnotations()
	if pod3Annotations == nil {
		pod3Annotations = map[string]string{}
	}
	pod3Annotations[annotationKey] = annotationValue
	pod3.SetAnnotations(pod3Annotations)

	type args struct {
		resource  schema.GroupVersionResource
		namespace string
		filters   *Filters
	}
	type listInput struct {
		options metav1.ListOptions
	}
	type listOuput struct {
		object runtime.Object
		err    error
	}
	tests := []struct {
		name      string
		setup     func(*dynamicfake.FakeDynamicClient)
		args      args
		listInput listInput
		listOuput listOuput
	}{
		{
			name: "list pods cluster-scoped",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod1, pod1.GetNamespace()))
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod2, pod2.GetNamespace()))
			},
			args: args{
				resource:  podGVR,
				namespace: "",
			},
			listInput: listInput{},
			listOuput: listOuput{
				object: &unstructured.UnstructuredList{
					Object: map[string]interface{}{
						"kind": "PodList",
						"metadata": map[string]interface{}{
							"continue":        "",
							"resourceVersion": "",
						},
						"apiVersion": "v1",
					},
					Items: []unstructured.Unstructured{
						*pod1.DeepCopy(),
						*pod2.DeepCopy(),
					},
				},
			},
		},
		{
			name: "list pods namespace-scoped",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod1, pod1.GetNamespace()))
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod2, pod2.GetNamespace()))
			},
			args: args{
				resource:  podGVR,
				namespace: pod1.GetNamespace(),
			},
			listInput: listInput{},
			listOuput: listOuput{
				object: &unstructured.UnstructuredList{
					Object: map[string]interface{}{
						"kind": "PodList",
						"metadata": map[string]interface{}{
							"continue":        "",
							"resourceVersion": "",
						},
						"apiVersion": "v1",
					},
					Items: []unstructured.Unstructured{
						*pod1.DeepCopy(),
					},
				},
			},
		},
		{
			name: "list pods label selector",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod1, pod1.GetNamespace()))
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod2, pod2.GetNamespace()))
			},
			args: args{
				resource:  podGVR,
				namespace: "",
				filters: &Filters{
					Labels: labels.SelectorFromSet(labels.Set{
						labelKey: labelValue,
					}),
				},
			},
			listInput: listInput{},
			listOuput: listOuput{
				object: &unstructured.UnstructuredList{
					Object: map[string]interface{}{
						"kind": "PodList",
						"metadata": map[string]interface{}{
							"continue":        "",
							"resourceVersion": "",
						},
						"apiVersion": "v1",
					},
					Items: []unstructured.Unstructured{
						*pod2.DeepCopy(),
					},
				},
			},
		},
		{
			name: "list pods field selector",
			setup: func(fakeClient *dynamicfake.FakeDynamicClient) {
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod2, pod2.GetNamespace()))
				require.NoError(t, fakeClient.Tracker().Create(podGVR, pod3, pod3.GetNamespace()))
			},
			args: args{
				resource:  podGVR,
				namespace: "",
				filters: &Filters{
					Fields: fields.SelectorFromSet(fields.Set{
						fmt.Sprintf("metadata.annotations.%s", annotationKey): annotationValue,
					}),
				},
			},
			listInput: listInput{},
			listOuput: listOuput{
				object: &unstructured.UnstructuredList{
					Object: map[string]interface{}{
						"kind": "PodList",
						"metadata": map[string]interface{}{
							"continue":        "",
							"resourceVersion": "",
						},
						"apiVersion": "v1",
					},
					Items: []unstructured.Unstructured{
						// FakeDynamicClient does not support field selectors or
						// specifying custom indexers to make them work.
						// TODO: Update FakeDynamicClient (client-go) to support indexers and field selectors
						*pod2.DeepCopy(), // Should not be returned
						*pod3.DeepCopy(),
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypes(metav1.SchemeGroupVersion, &metav1.Status{})

			// Register core v1 resources
			require.NoError(t, v1.AddToScheme(scheme))

			// Fake client that only knows about the types registered to the scheme
			fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

			// log fakeClient calls
			fakeClient.PrependReactor("*", "*", func(a clienttesting.Action) (bool, runtime.Object, error) {
				klog.Infof("FakeDynamicClient: %T{ Verb: %q, Resource: %q, Namespace: %q }",
					a, a.GetVerb(), a.GetResource().Resource, a.GetNamespace())
				return false, nil, nil
			})
			fakeClient.PrependWatchReactor("*", func(a clienttesting.Action) (bool, watch.Interface, error) {
				klog.Infof("FakeDynamicClient: %T{ Verb: %q, Resource: %q, Namespace: %q }",
					a, a.GetVerb(), a.GetResource().Resource, a.GetNamespace())
				return false, nil, nil
			})

			tc.setup(fakeClient)

			ctx := context.Background()
			lw := NewFilteredListWatchFromDynamicClient(ctx, fakeClient, tc.args.resource, tc.args.namespace, tc.args.filters)

			obj, err := lw.List(tc.listInput.options)
			testutil.AssertEqual(t, tc.listOuput.object, obj)
			if err != nil && tc.listOuput.err != nil {
				testutil.AssertEqual(t, testutil.EqualError(tc.listOuput.err), testutil.EqualError(err))
			} else {
				testutil.AssertEqual(t, tc.listOuput.err, err)
			}
		})
	}
}

func TestNewFilteredListWatchFromDynamicClientWatch(t *testing.T) {
	testTimeout := 10 * time.Second

	fakeMapper := testutil.NewFakeRESTMapper(
		v1.SchemeGroupVersion.WithKind("Pod"),
	)

	pod1 := yamlToUnstructured(t, pod1Yaml)
	pod1.SetNamespace("ns-1")
	// pod1ID := object.UnstructuredToObjMetadata(pod1)
	// pod1Current := yamlToUnstructured(t, pod1CurrentYaml)
	podGVR := getGVR(t, fakeMapper, pod1)

	pod1Current := yamlToUnstructured(t, pod1CurrentYaml)
	pod1Current.SetNamespace("ns-1")

	pod2 := pod1.DeepCopy()
	pod2.SetNamespace("ns-2")
	pod2.SetName("pod-2")

	labelKey := "example-key"
	labelValue := "example-value"
	pod2Labels := pod2.GetLabels()
	if pod2Labels == nil {
		pod2Labels = map[string]string{}
	}
	pod2Labels[labelKey] = labelValue
	pod2.SetLabels(pod2Labels)

	pod2Current := yamlToUnstructured(t, pod1CurrentYaml)
	pod2Current.SetNamespace("ns-2")
	pod2Current.SetName("pod-2")
	pod2Current.SetLabels(pod2Labels)

	pod3 := pod1.DeepCopy()
	pod3.SetNamespace("ns-2")
	pod3.SetName("pod-3")

	pod3Current := yamlToUnstructured(t, pod1CurrentYaml)
	pod3Current.SetNamespace("ns-2")
	pod3Current.SetName("pod-3")

	// nodeName is a valid server-side field selector
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/#supported-fields
	nodeNameFieldPath := "spec.nodeName"
	nodeNameFieldPathKeys := strings.Split(nodeNameFieldPath, ".")
	nodeNameValue := "example-node"
	require.NoError(t, unstructured.SetNestedField(pod3Current.Object, nodeNameValue, nodeNameFieldPathKeys...))

	type args struct {
		resource  schema.GroupVersionResource
		namespace string
		filters   *Filters
	}
	type watchInput struct {
		options metav1.ListOptions
	}
	type watchOuput struct {
		err error
	}
	tests := []struct {
		name           string
		setup          func(*dynamicfake.FakeDynamicClient)
		args           args
		watchInput     watchInput
		watchOuput     watchOuput
		clusterUpdates []func(*dynamicfake.FakeDynamicClient)
		expectedEvents []watch.Event
	}{
		{
			name: "watch pod cluster-scope",
			args: args{
				resource:  podGVR,
				namespace: "",
			},
			clusterUpdates: []func(fakeClient *dynamicfake.FakeDynamicClient){
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Create(podGVR, pod1, pod1.GetNamespace()))
				},
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Update(podGVR, pod1Current, pod1Current.GetNamespace()))
				},
			},
			expectedEvents: []watch.Event{
				{
					Type:   watch.Added,
					Object: pod1.DeepCopy(),
				},
				{
					Type:   watch.Modified,
					Object: pod1Current.DeepCopy(),
				},
			},
		},
		{
			name: "watch pods namespace-scoped",
			args: args{
				resource:  podGVR,
				namespace: pod1.GetNamespace(),
			},
			clusterUpdates: []func(fakeClient *dynamicfake.FakeDynamicClient){
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Create(podGVR, pod1, pod1.GetNamespace()))
				},
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Update(podGVR, pod1Current, pod1Current.GetNamespace()))
				},
			},
			expectedEvents: []watch.Event{
				{
					Type:   watch.Added,
					Object: pod1.DeepCopy(),
				},
				{
					Type:   watch.Modified,
					Object: pod1Current.DeepCopy(),
				},
			},
		},
		{
			name: "watch pods label selector",
			args: args{
				resource:  podGVR,
				namespace: "",
				filters: &Filters{
					Labels: labels.SelectorFromSet(labels.Set{
						labelKey: labelValue,
					}),
				},
			},
			// FakeDynamicClient doesn't implement watch restrictions (labels or fields),
			// so we have to fake a label selector by not sending those cluster updates.
			// TODO: Update FakeDynamicClient (client-go) to support watch restrictions.
			clusterUpdates: []func(fakeClient *dynamicfake.FakeDynamicClient){
				// func(fakeClient *dynamicfake.FakeDynamicClient) {
				// 	require.NoError(t, fakeClient.Tracker().Create(podGVR, pod1, pod1.GetNamespace()))
				// },
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Create(podGVR, pod2, pod2.GetNamespace()))
				},
				// func(fakeClient *dynamicfake.FakeDynamicClient) {
				// 	require.NoError(t, fakeClient.Tracker().Update(podGVR, pod1Current, pod1Current.GetNamespace()))
				// },
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Update(podGVR, pod2Current, pod2Current.GetNamespace()))
				},
			},
			expectedEvents: []watch.Event{
				{
					Type:   watch.Added,
					Object: pod2.DeepCopy(),
				},
				{
					Type:   watch.Modified,
					Object: pod2Current.DeepCopy(),
				},
			},
		},
		{
			name: "watch pods field selector",
			args: args{
				resource:  podGVR,
				namespace: "",
				filters: &Filters{
					Fields: fields.SelectorFromSet(fields.Set{
						nodeNameFieldPath: nodeNameValue,
					}),
				},
			},
			// FakeDynamicClient doesn't implement watch restrictions (labels or fields),
			// so we have to fake a field selector by not sending those cluster updates.
			// TODO: Update FakeDynamicClient (client-go) to support watch restrictions.
			clusterUpdates: []func(fakeClient *dynamicfake.FakeDynamicClient){
				// func(fakeClient *dynamicfake.FakeDynamicClient) {
				// 	require.NoError(t, fakeClient.Tracker().Create(podGVR, pod2, pod2.GetNamespace()))
				// },
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Create(podGVR, pod3, pod3.GetNamespace()))
				},
				// func(fakeClient *dynamicfake.FakeDynamicClient) {
				// 	require.NoError(t, fakeClient.Tracker().Update(podGVR, pod2Current, pod2Current.GetNamespace()))
				// },
				func(fakeClient *dynamicfake.FakeDynamicClient) {
					require.NoError(t, fakeClient.Tracker().Update(podGVR, pod3Current, pod3Current.GetNamespace()))
				},
			},
			expectedEvents: []watch.Event{
				// If FakeDynamicClient supported field selectors, the first
				// and only event seen would be when the spec.nodeName is set,
				// as an Added event.
				// {
				// 	Type:   watch.Added,
				// 	Object: pod3Current.DeepCopy(),
				// },
				{
					Type:   watch.Added,
					Object: pod3.DeepCopy(),
				},
				{
					Type:   watch.Modified,
					Object: pod3Current.DeepCopy(),
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypes(metav1.SchemeGroupVersion, &metav1.Status{})

			// Register core v1 resources
			require.NoError(t, v1.AddToScheme(scheme))

			// Fake client that only knows about the types registered to the scheme
			fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)

			// log fakeClient calls
			fakeClient.PrependReactor("*", "*", func(a clienttesting.Action) (bool, runtime.Object, error) {
				klog.V(3).Infof("FakeDynamicClient: %T{ Verb: %q, Resource: %q, Namespace: %q }",
					a, a.GetVerb(), a.GetResource().Resource, a.GetNamespace())
				return false, nil, nil
			})
			fakeClient.PrependWatchReactor("*", func(a clienttesting.Action) (bool, watch.Interface, error) {
				klog.V(3).Infof("FakeDynamicClient: %T{ Verb: %q, Resource: %q, Namespace: %q }",
					a, a.GetVerb(), a.GetResource().Resource, a.GetNamespace())
				return false, nil, nil
			})

			if tc.setup != nil {
				tc.setup(fakeClient)
			}

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			lw := NewFilteredListWatchFromDynamicClient(ctx, fakeClient, tc.args.resource, tc.args.namespace, tc.args.filters)

			watcher, err := lw.Watch(tc.watchInput.options)

			if err != nil && tc.watchOuput.err != nil {
				testutil.AssertEqual(t, testutil.EqualError(tc.watchOuput.err), testutil.EqualError(err))
			} else {
				testutil.AssertEqual(t, tc.watchOuput.err, err)
			}

			nextCh := make(chan struct{})
			defer close(nextCh)

			// Synchronize event consumption and production for predictable test results.
			go func() {
				// Wait for start event
				<-nextCh
				for _, update := range tc.clusterUpdates {
					update(fakeClient)
					<-nextCh
				}
				// Stop the watcher
				watcher.Stop()
			}()

			// Start server updates
			nextCh <- struct{}{}

			receivedEvents := []watch.Event{}
			func() {
				doneCh := ctx.Done()
				resultCh := watcher.ResultChan()
				for {
					select {
					case <-doneCh:
						t.Errorf("test timed out before event channel closed")
						return
					case e, open := <-resultCh:
						if !open {
							klog.V(3).Info("event channel closed")
							return
						}
						klog.V(3).Infof("event received: %#v", e)
						receivedEvents = append(receivedEvents, e)
						// Trigger next server update
						nextCh <- struct{}{}
					}
				}
			}()
			testutil.AssertEqual(t, tc.expectedEvents, receivedEvents)
		})
	}
}

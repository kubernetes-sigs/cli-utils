// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/testapigroup"
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

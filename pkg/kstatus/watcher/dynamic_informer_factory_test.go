// Copyright 2022 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"net/http"
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
					name := "" // unused by LIST requests
					// The apisevrer confusingly does not return apierrors.NewNotFound,
					// which has a nice constant for its error message.
					// err = apierrors.NewNotFound(exampleGR, name)
					// Instead it uses apierrors.NewGenericServerResponse, which uses
					// a hard-coded error message.
					err = apierrors.NewGenericServerResponse(http.StatusNotFound, "list", exampleGR, name, "unused", -1, false)
					return true, nil, err
				})
			},
			errorHandler: func(t *testing.T, err error) {
				switch {
				case apierrors.IsNotFound(err):
					// If we got this error, something changed in the apiserver or
					// client. If the client changed, it might be safe to stop parsing
					// the error string.
					t.Errorf("Expected untyped NotFound error, but got typed NotFound error: %v", err)
				case containsNotFoundMessage(err):
					// This is the expected hack, because the Informer/Reflector
					// doesn't wrap the error with "%w".
					t.Logf("Received expected untyped NotFound error: %v", err)
				default:
					// If we got this error, the test is probably broken.
					t.Errorf("Expected untyped NotFound error, but got a different error: %v", err)
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
					name := "" // unused by LIST requests
					// The apisevrer confusingly does not return apierrors.NewNotFound,
					// which has a nice constant for its error message.
					// err = apierrors.NewNotFound(exampleGR, name)
					// Instead it uses apierrors.NewGenericServerResponse, which uses
					// a hard-coded error message.
					err = apierrors.NewGenericServerResponse(http.StatusNotFound, "list", exampleGR, name, "unused", -1, false)
					return true, nil, err
				})
			},
			errorHandler: func(t *testing.T, err error) {
				switch {
				case apierrors.IsNotFound(err):
					// This is the expected behavior, because the
					// Informer/Reflector DOES wrap watch errors
					t.Logf("Received expected untyped NotFound error: %v", err)
				case containsNotFoundMessage(err):
					// If this happens, there was a regression.
					// Watch errors are expected to be wrapped with "%w"
					t.Errorf("Expected typed NotFound error, but got untyped NotFound error: %v", err)
				default:
					// If we got this error, the test is probably broken.
					t.Errorf("Expected untyped NotFound error, but got a different error: %v", err)
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

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package clusterreader

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/cli-utils/pkg/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	deploymentGVK = appsv1.SchemeGroupVersion.WithKind("Deployment")
	rsGVK         = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	podGVK        = v1.SchemeGroupVersion.WithKind("Pod")
	crdGVK        = schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}
)

func TestSync(t *testing.T) {
	// Use a custom Asserter to customize the comparison options
	asserter := testutil.NewAsserter(
		cmpopts.EquateErrors(),
		gkNamespaceComparer(),
		cacheEntryComparer(),
	)

	testCases := map[string]struct {
		identifiers    object.ObjMetadataSet
		clusterObjs    map[gkNamespace][]unstructured.Unstructured
		expectedSynced []gkNamespace
		expectedCached map[gkNamespace]cacheEntry
	}{
		"no identifiers": {
			identifiers:    object.ObjMetadataSet{},
			expectedCached: map[gkNamespace]cacheEntry{},
		},
		"same GVK in multiple namespaces": {
			identifiers: object.ObjMetadataSet{
				{
					GroupKind: deploymentGVK.GroupKind(),
					Name:      "deployment",
					Namespace: "Foo",
				},
				{
					GroupKind: deploymentGVK.GroupKind(),
					Name:      "deployment",
					Namespace: "Bar",
				},
			},
			clusterObjs: map[gkNamespace][]unstructured.Unstructured{
				{GroupKind: deploymentGVK.GroupKind(), Namespace: "Foo"}: {
					{
						Object: map[string]any{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]any{
								"name":      "deployment-1",
								"namespace": "Foo",
							},
						},
					},
				},
				{GroupKind: deploymentGVK.GroupKind(), Namespace: "Bar"}: {
					{
						Object: map[string]any{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]any{
								"name":      "deployment-2",
								"namespace": "Bar",
							},
						},
					},
				},
			},
			expectedSynced: []gkNamespace{
				{GroupKind: deploymentGVK.GroupKind(), Namespace: "Foo"},
				{GroupKind: rsGVK.GroupKind(), Namespace: "Foo"},
				{GroupKind: podGVK.GroupKind(), Namespace: "Foo"},
				{GroupKind: deploymentGVK.GroupKind(), Namespace: "Bar"},
				{GroupKind: rsGVK.GroupKind(), Namespace: "Bar"},
				{GroupKind: podGVK.GroupKind(), Namespace: "Bar"},
			},
			expectedCached: map[gkNamespace]cacheEntry{
				{GroupKind: deploymentGVK.GroupKind(), Namespace: "Foo"}: {
					resources: unstructured.UnstructuredList{
						Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment"},
						Items: []unstructured.Unstructured{
							{
								Object: map[string]any{
									"apiVersion": "apps/v1",
									"kind":       "Deployment",
									"metadata": map[string]any{
										"name":      "deployment-1",
										"namespace": "Foo",
									},
								},
							},
						},
					},
				},
				{GroupKind: rsGVK.GroupKind(), Namespace: "Foo"}: {
					resources: unstructured.UnstructuredList{
						Object: map[string]any{"apiVersion": "apps/v1", "kind": "ReplicaSet"},
					},
				},
				{GroupKind: podGVK.GroupKind(), Namespace: "Foo"}: {
					resources: unstructured.UnstructuredList{
						Object: map[string]any{"apiVersion": "v1", "kind": "Pod"},
					},
				},
				{GroupKind: deploymentGVK.GroupKind(), Namespace: "Bar"}: {
					resources: unstructured.UnstructuredList{
						Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment"},
						Items: []unstructured.Unstructured{
							{
								Object: map[string]any{
									"apiVersion": "apps/v1",
									"kind":       "Deployment",
									"metadata": map[string]any{
										"name":      "deployment-2",
										"namespace": "Bar",
									},
								},
							},
						},
					},
				},
				{GroupKind: rsGVK.GroupKind(), Namespace: "Bar"}: {
					resources: unstructured.UnstructuredList{
						Object: map[string]any{"apiVersion": "apps/v1", "kind": "ReplicaSet"},
					},
				},
				{GroupKind: podGVK.GroupKind(), Namespace: "Bar"}: {
					resources: unstructured.UnstructuredList{
						Object: map[string]any{"apiVersion": "v1", "kind": "Pod"},
					},
				},
			},
		},
	}

	barPodGKN := gkNamespace{GroupKind: podGVK.GroupKind(), Namespace: "Bar"}
	// 1001 = 3 pages of 500
	barObjs := make([]unstructured.Unstructured, 1001)
	for i := 0; i < len(barObjs); i++ {
		barObjs[i] = unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": podGVK.GroupVersion().String(),
				"kind":       podGVK.Kind,
				"metadata": map[string]any{
					"name":      fmt.Sprintf("pod-%d", i),
					"namespace": barPodGKN.Namespace,
				},
			},
		}
	}
	testCases["paginated"] = struct {
		identifiers    object.ObjMetadataSet
		clusterObjs    map[gkNamespace][]unstructured.Unstructured
		expectedSynced []gkNamespace
		expectedCached map[gkNamespace]cacheEntry
	}{
		identifiers: object.ObjMetadataSet{
			// any one pod
			{
				GroupKind: podGVK.GroupKind(),
				Name:      "pod-99",
				Namespace: barPodGKN.Namespace,
			},
		},
		clusterObjs: map[gkNamespace][]unstructured.Unstructured{
			barPodGKN: barObjs,
		},
		expectedSynced: []gkNamespace{
			// expect 3 paginated calls to LIST
			barPodGKN,
			barPodGKN,
			barPodGKN,
		},
		expectedCached: map[gkNamespace]cacheEntry{
			barPodGKN: {
				resources: unstructured.UnstructuredList{
					Object: map[string]any{
						"apiVersion": podGVK.GroupVersion().String(),
						"kind":       podGVK.Kind,
					},
					// all the deployments in the same namespace
					Items: barObjs,
				},
			},
		},
	}

	fakeMapper := testutil.NewFakeRESTMapper(
		deploymentGVK,
		rsGVK,
		v1.SchemeGroupVersion.WithKind("Pod"),
	)

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			fakeReader := &fakeReader{
				clusterObjs: tc.clusterObjs,
			}

			clusterReader, err := newCachingClusterReader(fakeReader, fakeMapper, tc.identifiers)
			require.NoError(t, err)

			err = clusterReader.Sync(context.Background())
			require.NoError(t, err)

			synced := fakeReader.syncedGVKNamespaces
			sortGVKNamespaces(synced)
			expectedSynced := tc.expectedSynced
			sortGVKNamespaces(expectedSynced)
			asserter.Equal(t, expectedSynced, synced)
			asserter.Equal(t, tc.expectedCached, clusterReader.cache)
		})
	}
}

func TestSync_Errors(t *testing.T) {
	testCases := map[string]struct {
		mapper          meta.RESTMapper
		readerError     error
		expectSyncError bool
		cacheError      bool
		cacheErrorText  string
	}{
		"mapping and reader are successful": {
			mapper: testutil.NewFakeRESTMapper(
				crdGVK,
			),
			readerError:     nil,
			expectSyncError: false,
			cacheError:      false,
		},
		"reader returns NotFound error": {
			mapper: testutil.NewFakeRESTMapper(
				crdGVK,
			),
			readerError: errors.NewNotFound(schema.GroupResource{
				Group:    "apiextensions.k8s.io",
				Resource: "customresourcedefinitions",
			}, "my-crd"),
			expectSyncError: false,
			cacheError:      true,
			cacheErrorText:  `customresourcedefinitions.apiextensions.k8s.io "my-crd" not found`,
		},
		"reader returns other error": {
			mapper: testutil.NewFakeRESTMapper(
				crdGVK,
			),
			readerError:     errors.NewInternalError(fmt.Errorf("testing")),
			expectSyncError: false,
			cacheError:      true,
			cacheErrorText:  "Internal error occurred: testing",
		},
		"mapping not found": {
			mapper:          testutil.NewFakeRESTMapper(),
			expectSyncError: false,
			cacheError:      true,
			cacheErrorText:  `no matches for kind "CustomResourceDefinition" in group "apiextensions.k8s.io"`,
		},
	}

	for tn, tc := range testCases {
		t.Run(tn, func(t *testing.T) {
			identifiers := object.ObjMetadataSet{
				{
					Name: "my-crd",
					GroupKind: schema.GroupKind{
						Group: "apiextensions.k8s.io",
						Kind:  "CustomResourceDefinition",
					},
				},
			}

			fakeReader := &fakeReader{
				err: tc.readerError,
			}

			clusterReader, err := newCachingClusterReader(fakeReader, tc.mapper, identifiers)
			require.NoError(t, err)

			err = clusterReader.Sync(context.Background())

			if tc.expectSyncError {
				assert.Equal(t, tc.readerError, err)
				return
			}
			require.NoError(t, err)

			cacheEntry, found := clusterReader.cache[gkNamespace{
				GroupKind: crdGVK.GroupKind(),
			}]
			require.True(t, found)
			if tc.cacheError {
				assert.EqualError(t, cacheEntry.err, tc.cacheErrorText)
			}
		})
	}
}

// newCachingClusterReader creates a new CachingClusterReader and returns it as the concrete
// type instead of engine.ClusterReader.
func newCachingClusterReader(reader client.Reader, mapper meta.RESTMapper, identifiers object.ObjMetadataSet) (*CachingClusterReader, error) {
	r, err := NewCachingClusterReader(reader, mapper, identifiers)
	if err != nil {
		return nil, err
	}
	return r.(*CachingClusterReader), nil
}

func sortGVKNamespaces(gvkNamespaces []gkNamespace) {
	sort.Slice(gvkNamespaces, func(i, j int) bool {
		if gvkNamespaces[i].GroupKind.String() != gvkNamespaces[j].GroupKind.String() {
			return gvkNamespaces[i].GroupKind.String() < gvkNamespaces[j].GroupKind.String()
		}
		return gvkNamespaces[i].Namespace < gvkNamespaces[j].Namespace
	})
}

type fakeReader struct {
	clusterObjs         map[gkNamespace][]unstructured.Unstructured
	syncedGVKNamespaces []gkNamespace
	err                 error
}

func (f *fakeReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object, opts ...client.GetOption) error {
	return nil
}

//nolint:gocritic
func (f *fakeReader) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := &client.ListOptions{}
	listOpts.ApplyOptions(opts)

	gvk := list.GetObjectKind().GroupVersionKind()
	query := gkNamespace{
		GroupKind: gvk.GroupKind(),
		Namespace: listOpts.Namespace,
	}

	f.syncedGVKNamespaces = append(f.syncedGVKNamespaces, query)

	if f.err != nil {
		return f.err
	}

	results, ok := f.clusterObjs[query]
	if !ok {
		// no results
		return nil
	}

	uList, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return fmt.Errorf("unexpected list type: %T", list)
	}

	if listOpts.Limit > 0 && len(results) > 0 {
		// return paginated results from Continue to Continue + Limit
		start := int64(0)
		if listOpts.Continue != "" {
			var err error
			start, err = strconv.ParseInt(listOpts.Continue, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid continue value: %q", listOpts.Continue)
			}
		}
		end := start + listOpts.Limit
		maxResult := int64(len(results))
		if end > maxResult {
			end = maxResult
		} else {
			// set continue if more results are available
			uList.SetContinue(strconv.FormatInt(end, 10))
		}
		uList.Items = append(uList.Items, results[start:end]...)
	} else {
		uList.Items = results
	}

	return nil
}

func gkNamespaceComparer() cmp.Option {
	return cmp.Comparer(func(x, y gkNamespace) bool {
		return x.GroupKind == y.GroupKind &&
			x.Namespace == y.Namespace
	})
}

func cacheEntryComparer() cmp.Option {
	return cmp.Comparer(func(x, y cacheEntry) bool {
		if x.err != y.err {
			return false
		}
		xBytes, err := json.Marshal(x.resources)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal item x to json: %v", err))
		}
		yBytes, err := json.Marshal(y.resources)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal item y to json: %v", err))
		}
		return string(xBytes) == string(yBytes)
	})
}

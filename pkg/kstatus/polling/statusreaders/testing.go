// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package statusreaders

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/testutil"
	"sigs.k8s.io/cli-utils/pkg/object"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeClusterReader struct {
	testutil.NoopClusterReader

	getResource *unstructured.Unstructured
	getErr      error

	listResources *unstructured.UnstructuredList
	listErr       error
}

func (f *fakeClusterReader) Get(_ context.Context, _ client.ObjectKey, u *unstructured.Unstructured) error {
	if f.getResource != nil {
		u.Object = f.getResource.Object
	}
	return f.getErr
}

func (f *fakeClusterReader) ListNamespaceScoped(_ context.Context, list *unstructured.UnstructuredList, _ string, _ labels.Selector) error {
	if f.listResources != nil {
		list.Items = f.listResources.Items
	}
	return f.listErr
}

type fakeStatusReader struct{}

func (f *fakeStatusReader) ReadStatus(_ context.Context, _ object.ObjMetadata) *event.ResourceStatus {
	return nil
}

func (f *fakeStatusReader) ReadStatusForObject(_ context.Context, object *unstructured.Unstructured) *event.ResourceStatus {
	identifier := toIdentifier(object)
	return &event.ResourceStatus{
		Identifier: identifier,
	}
}

func fakeStatusForGenResourcesFunc(resourceStatuses event.ResourceStatuses, err error) statusForGenResourcesFunc {
	return func(_ context.Context, _ meta.RESTMapper, _ engine.ClusterReader, _ resourceTypeStatusReader,
		_ *unstructured.Unstructured, _ schema.GroupKind, _ ...string) (event.ResourceStatuses, error) {
		return resourceStatuses, err
	}
}

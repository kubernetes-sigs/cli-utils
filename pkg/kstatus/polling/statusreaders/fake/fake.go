// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type StatusReader struct{}

func (f *StatusReader) Supports(schema.GroupKind) bool {
	return true
}

func (f *StatusReader) ReadStatus(_ context.Context, _ engine.ClusterReader, _ object.ObjMetadata) (*event.ResourceStatus, error) {
	return nil, nil
}

func (f *StatusReader) ReadStatusForObject(_ context.Context, _ engine.ClusterReader, obj *unstructured.Unstructured) (*event.ResourceStatus, error) {
	identifier := object.UnstructuredToObjMetadata(obj)
	return &event.ResourceStatus{
		Identifier: identifier,
	}, nil
}

// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func YamlToUnstructured(t *testing.T, yml string) *unstructured.Unstructured {
	m := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(yml), &m)
	if err != nil {
		t.Fatalf("error parsing yaml: %v", err)
		return nil
	}
	return &unstructured.Unstructured{Object: m}
}

func NewNoopClusterReader() *NoopClusterReader {
	return &NoopClusterReader{}
}

type NoopClusterReader struct{}

func (n *NoopClusterReader) Get(_ context.Context, _ client.ObjectKey, _ *unstructured.Unstructured) error {
	return nil
}

func (n *NoopClusterReader) ListNamespaceScoped(_ context.Context, _ *unstructured.UnstructuredList,
	_ string, _ labels.Selector) error {
	return nil
}

func (n *NoopClusterReader) ListClusterScoped(_ context.Context, _ *unstructured.UnstructuredList,
	_ labels.Selector) error {
	return nil
}

func (n *NoopClusterReader) Sync(_ context.Context) error {
	return nil
}

package testutil

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func NewFakeRESTMapper(gvks ...schema.GroupVersionKind) meta.RESTMapper {
	var groupVersions []schema.GroupVersion
	for _, gvk := range gvks {
		groupVersions = append(groupVersions, gvk.GroupVersion())
	}
	mapper := meta.NewDefaultRESTMapper(groupVersions)
	for _, gvk := range gvks {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	return mapper
}

func NewNoopObserverReader() *NoopObserverReader {
	return &NoopObserverReader{}
}

type NoopObserverReader struct{}

func (n *NoopObserverReader) Get(_ context.Context, _ client.ObjectKey, _ *unstructured.Unstructured) error {
	return nil
}

func (n *NoopObserverReader) ListNamespaceScoped(_ context.Context, _ *unstructured.UnstructuredList, _ string, _ labels.Selector) error {
	return nil
}

func (n *NoopObserverReader) ListClusterScoped(_ context.Context, _ *unstructured.UnstructuredList, _ labels.Selector) error {
	return nil
}

func (n *NoopObserverReader) Sync(_ context.Context) error {
	return nil
}

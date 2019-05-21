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

package pkg

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kustomize/pkg/inventory"
)

func setupResources() []*unstructured.Unstructured {
	r1 := &unstructured.Unstructured{}
	r1.SetGroupVersionKind(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	})
	r1.SetName("cm1")
	r1.SetNamespace("default")
	r2 := &unstructured.Unstructured{}
	r2.SetGroupVersionKind(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	})
	r2.SetName("cm2")
	r2.SetNamespace("default")
	r2.SetAnnotations(map[string]string{
		inventory.InventoryAnnotation:     "{}",
		inventory.InventoryHashAnnotation: "1234567",
	})
	return []*unstructured.Unstructured{r1, r2}
}

func TestCmd(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd, err := InitializeCmd(buf, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	cmList := &unstructured.UnstructuredList{}
	cmList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ConfigMapList",
		Version: "v1",
	})

	c := cmd.Applier.DynamicClient
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 0)

	resources := setupResources()
	err = cmd.Apply(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)

	err = cmd.Prune(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 2)

	err = cmd.Delete(resources)
	assert.NoError(t, err)
	err = c.List(context.Background(), cmList, "default", nil)
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 0)
}

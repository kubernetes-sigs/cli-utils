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

package delete_test

import (
	"bytes"
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/delete"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

func TestDeleteEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	d, done, err := wiretest.InitializeDelete(clik8s.ResourceConfigs(nil), &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := d.Do()
	assert.NoError(t, err)
	assert.Equal(t, delete.Result{}, r)
}

func TestPrune(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := wiretest.InitializeKustomization()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	a, donea, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	assert.NoError(t, err)
	defer donea()
	_, err = a.Do()
	assert.NoError(t, err)
	updatedObjects, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	a.Resources = updatedObjects
	_, err = a.Do()
	assert.NoError(t, err)

	cmList := &unstructured.UnstructuredList{}
	cmList.SetGroupVersionKind(schema.GroupVersionKind{
		Kind:    "ConfigMapList",
		Version: "v1",
	})
	err = a.DynamicClient.List(context.Background(), cmList, "default", metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 3)

	d, doned, err := wiretest.InitializeDelete(updatedObjects, &object.Commit{}, buf)
	defer doned()
	assert.NoError(t, err)
	d.DynamicClient = a.DynamicClient
	_, err = d.Do()
	assert.NoError(t, err)

	err = d.DynamicClient.List(context.Background(), cmList, "default", metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, len(cmList.Items), 0)
}

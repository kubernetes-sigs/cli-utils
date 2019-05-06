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

package resourceconfig_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"sigs.k8s.io/kustomize/pkg/inventory"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

func TestKustomizeProvider(t *testing.T) {
	kp := wiretest.InitializConfigProvider()
	objects, err := kp.GetConfig("github.com/kubernetes-sigs/kustomize//examples/multibases")
	assert.NoError(t, err)
	assert.NotEmpty(t, objects)
	assert.Equal(t, len(objects), 3)
}

func setupKustomize(t *testing.T) string {
	f, err := ioutil.TempDir("/tmp", "TestApply")
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(f, "kustomization.yaml"), []byte(`
configMapGenerator:
- name: testmap
namespace: default
inventory:
  type: ConfigMap
  configMap:
    name: root-cm
    namespace: default
`), 0644)
	assert.NoError(t, err)
	return f
}

func TestKustomizeProvider2(t *testing.T) {
	f := setupKustomize(t)
	kp := wiretest.InitializConfigProvider()
	objects, err := kp.GetConfig(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, objects)
	assert.Equal(t, len(objects), 2)
	pobject, err := kp.GetPruneConfig(f)
	assert.NoError(t, err)
	assert.NotEmpty(t, pobject)
	assert.NotNil(t, pobject)
	inv := inventory.NewInventory()
	inv.LoadFromAnnotation(pobject.GetAnnotations())
	assert.Equal(t, len(inv.Current), 1)
}

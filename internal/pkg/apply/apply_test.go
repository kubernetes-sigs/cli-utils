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

package apply_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/cli-experimental/internal/pkg/constants"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/apply"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
)

func TestApplyEmpty(t *testing.T) {
	buf := new(bytes.Buffer)
	a, done, err := wiretest.InitializeApply(clik8s.ResourceConfigs(nil), &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{}, r)
}

func TestApply(t *testing.T) {
	buf := new(bytes.Buffer)
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := wiretest.InitializeKustomization()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)

	a, done, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{objects}, r)

	updatedObjects, err := kp.GetConfig(fs[1])
	a.Resources = updatedObjects
	assert.NoError(t, err)
	r, err = a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{updatedObjects}, r)
}

func InitializeKustomizationWithPresence() ([]string, func(), error) {
	f1, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f1, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: test-map

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

resources:
- apply-service.yaml

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(filepath.Join(f1, "apply-service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	f2, err := ioutil.TempDir("/tmp", "TestApply")
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(filepath.Join(f2, "kustomization.yaml"), []byte(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: test-map
  literals:
  - foo=bar

inventory:
  type: ConfigMap
  configMap:
    name: inventory
    namespace: default

resources:
- not-apply-service.yaml

namespace: default
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(filepath.Join(f2, "not-apply-service.yaml"), []byte(`
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    kubectl.kubernetes.io/presence: EnsureDoesNotExist
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 9376
`), 0644)
	if err != nil {
		return nil, nil, err
	}

	return []string{f1, f2}, func() {
		os.RemoveAll(f1)
		os.RemoveAll(f2)
	}, nil
}

/* TestApplyWithPresenceAnnotation verifies that Apply does not
   delete resources but ignores deleted resource. Deletion should be delegated.
   It takes following steps
   1. create a Kustomization with
         a ConfigMapGenerator
         a Service
         an inventory ConfigMap
   2. apply the kustomization
   3. confirm that there are
         1 Service
   3. update the service to have the annotation EnsureDoesNotExist
   4. apply the kustomization again
   5. confirm that there are
         1 Service
   6. confirm that the existing service not updated with the annotation
   7. manually delete the resource, re-apply
      verify it is not re-created
*/
func TestApplyWithPresenceAnnotation(t *testing.T) {
	buf := new(bytes.Buffer)

	// set up a kustomization
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := InitializeKustomizationWithPresence()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	// confirm the loaded resources containing a service
	// without the annotation for EnsureDoesNotExist
	objects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	service := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"protocol":   "TCP",
						"targetPort": int64(9376)},
				},
				"selector": map[string]interface{}{"app": "MyApp"},
			},
		},
	}
	assert.NoError(t, err)
	assert.Contains(t, objects, service)

	// Initialize the Apply object
	a, done, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)

	// Apply the first kustomization
	// Confirm there is one Service object
	// Confirm the Service object is the expected one
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{objects}, r)
	liveService := service.DeepCopy()
	exist, err := util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, true, exist)
	assert.Equal(t, false, util.HasAnnotation(liveService, constants.Presence))

	// Update the kustomization
	// by adding the annotation EnsureDoesNotExist to the Service object
	// Confirm there is a Service resource with the annotation EnsureDoesNotExist
	updatedObjects, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	updatedService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/presence": "EnsureDoesNotExist",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"protocol":   "TCP",
						"targetPort": int64(9376)},
				},
				"selector": map[string]interface{}{"app": "MyApp"},
			},
		},
	}
	assert.Contains(t, updatedObjects, updatedService)
	assert.NotEqual(t, service, updatedService)

	// Apply the second kustomization
	// Confirm that the Service object is not updated
	a.Resources = updatedObjects
	r, err = a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{updatedObjects}, r)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.Equal(t, true, exist)
	assert.NoError(t, err)
	assert.Equal(t, false, util.HasAnnotation(liveService, constants.Presence))

	// Delete the Service object
	// Reapply it and verify
	// the Service is not re-created
	err = a.DynamicClient.Delete(context.Background(), liveService, nil)
	assert.NoError(t, err)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, false, exist)

	_, err = a.Do()
	assert.NoError(t, err)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, false, exist)
}

/* TestApplyWithPresenceAnnotationOrder2 verifies that Apply does not
   create resources with the EnsureDoesNotExist annotation.
   It takes the following steps
   1. create a Kustomization with
         a ConfigMapGenerator
         a Service with the annotation EnsureDoesNotExist
         an inventory ConfigMap
   2. apply the kustomization
   3. confirm that there are
         0 Service
   3. Delete the annotation EnsureDoesNotExist from the service
   4. apply the kustomization again
   5. confirm that there are
         1 Service
*/
func TestApplyWithPresenceAnnotationOrder2(t *testing.T) {
	buf := new(bytes.Buffer)

	// set up a kustomization
	kp := wiretest.InitializConfigProvider()
	fs, cleanup, err := InitializeKustomizationWithPresence()
	defer cleanup()
	assert.NoError(t, err)
	assert.Equal(t, len(fs), 2)

	// confirm the loaded resources containing a service
	// with the annotation for EnsureDoesNotExist
	objects, err := kp.GetConfig(fs[1])
	assert.NoError(t, err)
	service := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"kubectl.kubernetes.io/presence": "EnsureDoesNotExist",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"protocol":   "TCP",
						"targetPort": int64(9376)},
				},
				"selector": map[string]interface{}{"app": "MyApp"},
			},
		},
	}
	assert.NoError(t, err)
	assert.Contains(t, objects, service)

	// Initialize the Apply object
	a, done, err := wiretest.InitializeApply(objects, &object.Commit{}, buf)
	defer done()
	assert.NoError(t, err)

	// Apply the first kustomization
	// Confirm that the Service object is not created
	r, err := a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{objects}, r)
	liveService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
			},
		},
	}
	exist, err := util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.NoError(t, err)
	assert.Equal(t, false, exist)

	// Update the kustomization
	// by removing the annotation EnsureDoesNotExist from the Service object
	// Confirm there is a Service resource without the annotation EnsureDoesNotExist
	updatedObjects, err := kp.GetConfig(fs[0])
	assert.NoError(t, err)
	updatedService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"protocol":   "TCP",
						"targetPort": int64(9376)},
				},
				"selector": map[string]interface{}{"app": "MyApp"},
			},
		},
	}
	assert.Contains(t, updatedObjects, updatedService)
	assert.NotEqual(t, service, updatedService)

	// Apply the second kustomization
	// Confirm that the Service object is created
	// and without the annotation
	a.Resources = updatedObjects
	r, err = a.Do()
	assert.NoError(t, err)
	assert.Equal(t, apply.Result{updatedObjects}, r)
	exist, err = util.ObjectExist(a.DynamicClient, context.Background(), liveService)
	assert.Equal(t, true, exist)
	assert.NoError(t, err)
	assert.Equal(t, false, util.HasAnnotation(liveService, constants.Presence))
}

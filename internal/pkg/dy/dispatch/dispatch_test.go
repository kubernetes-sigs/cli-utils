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

package dispatch_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"sigs.k8s.io/cli-experimental/internal/pkg/dy/dispatch"

	v1 "k8s.io/api/apps/v1"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/parse"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
	"sigs.k8s.io/yaml"
)

var templ = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: foo
  name: foo
  namespace:
spec:
  replicas: 2
  selector:
    matchLabels:
      app: foo
  template:
    metadata:
      labels:
        app: foo
    spec:
      containers:
      - image: nginx
        name: foo
---
`

var instance *dispatch.Dispatcher
var buf = &bytes.Buffer{}

func TestMain(m *testing.M) {
	var stop func()
	var err error

	instance, stop, err = wiretest.InitializeDispatcher(buf)
	if err != nil {
		os.Exit(1)
	}
	defer stop()
	os.Exit(m.Run())
}

// TestDispatchCreate tests that a CreateResource operation succeeds
func TestDispatchCreate(t *testing.T) {
	buf.Reset()

	namespace := "create"
	expected := strings.Replace(templ, "namespace:", "namespace: "+namespace, -1)
	name := "foo"
	replicas := int32(2)
	image := "nginx"
	dryrun := true
	ns := &corev1.Namespace{}
	ns.Name = namespace
	_, err := instance.KubernetesClient.CoreV1().Namespaces().Create(ns)
	assert.NoError(t, err)

	req := &v1alpha1.ResourceCommand{
		Requests: []v1alpha1.ResourceRequest{
			{
				Version:   "v1",
				Group:     "apps",
				Resource:  "deployments",
				Operation: v1alpha1.CreateResource,
				BodyTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{index .Flags.Strings "name"}}
  namespace: {{index .Flags.Strings "namespace"}}
  labels:
    app: {{index .Flags.Strings "name"}}
spec:
  replicas: {{index .Flags.Ints "replicas"}}
  selector:
    matchLabels:
      app: {{index .Flags.Strings "name"}}
  template:
    metadata:
      labels:
        app: {{index .Flags.Strings "name"}}
    spec:
      containers:
      - name: {{index .Flags.Strings "name"}}
        image: {{index .Flags.Strings "image"}}
`,
				SaveResponseValues: []v1alpha1.ResponseValue{
					{
						JSONPath: "{.metadata.name}",
						Name:     "name",
					},
					{
						JSONPath: "{.spec.replicas}",
						Name:     "replicas",
					},
				},
			},
		},
	}
	values := parse.Values{
		Flags: parse.Flags{
			Strings: map[string]*string{
				"name":      &name,
				"namespace": &namespace,
				"image":     &image,
			},
			Ints: map[string]*int32{
				"replicas": &replicas,
			},
			Bools: map[string]*bool{
				"dry-run": &dryrun,
			},
		},
	}

	// Dry Run dispatch
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	assert.Equal(t, expected, buf.String())
	d, err := instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.Empty(t, d)

	// Actually dispatch
	dryrun = false
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	d, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, d)
	assert.NotEmpty(t, &values.Responses.Strings)
	assert.Equal(t, "foo", *values.Responses.Strings["name"])
	assert.Equal(t, "2", *values.Responses.Strings["replicas"])
}

// TestDispatchUpdate tests that an UpdateResource operation succeeds
func TestDispatchUpdate(t *testing.T) {
	buf.Reset()

	namespace := "update"
	expected := strings.Replace(templ, "namespace:", "namespace: "+namespace, -1)
	name := "foo"
	replicas := int32(3)
	image := "nginx"
	dryrun := true
	ns := &corev1.Namespace{}
	ns.Name = namespace
	_, err := instance.KubernetesClient.CoreV1().Namespaces().Create(ns)
	assert.NoError(t, err)

	createD := &v1.Deployment{}
	err = yaml.Unmarshal([]byte(expected), createD)
	assert.NoError(t, err)
	_, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Create(createD)
	assert.NoError(t, err)

	req := &v1alpha1.ResourceCommand{
		Requests: []v1alpha1.ResourceRequest{
			{
				Version:   "v1",
				Group:     "apps",
				Resource:  "deployments",
				Operation: v1alpha1.UpdateResource,
				BodyTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{index .Flags.Strings "name"}}
  namespace: {{index .Flags.Strings "namespace"}}
  labels:
    app: {{index .Flags.Strings "name"}}
spec:
  replicas: {{index .Flags.Ints "replicas"}}
  selector:
    matchLabels:
      app: {{index .Flags.Strings "name"}}
  template:
    metadata:
      labels:
        app: {{index .Flags.Strings "name"}}
    spec:
      containers:
      - name: {{index .Flags.Strings "name"}}
        image: {{index .Flags.Strings "image"}}
`,
				SaveResponseValues: []v1alpha1.ResponseValue{
					{
						JSONPath: "{.metadata.name}",
						Name:     "name",
					},
					{
						JSONPath: "{.spec.replicas}",
						Name:     "replicas",
					},
				},
			},
		},
	}
	values := parse.Values{
		Flags: parse.Flags{
			Strings: map[string]*string{
				"name":      &name,
				"namespace": &namespace,
				"image":     &image,
			},
			Ints: map[string]*int32{
				"replicas": &replicas,
			},
			Bools: map[string]*bool{
				"dry-run": &dryrun,
			},
		},
	}

	// Dry Run dispatch
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	d, err := instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, int32(2), *d.Spec.Replicas)

	// Actually dispatch
	dryrun = false
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	d, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, int32(3), *d.Spec.Replicas)
	assert.NotEmpty(t, &values.Responses.Strings)
	assert.Equal(t, "foo", *values.Responses.Strings["name"])
	assert.Equal(t, "3", *values.Responses.Strings["replicas"])
}

// TestDispatchDelete tests that a DeleteResource operation succeeds
func TestDispatchDelete(t *testing.T) {
	buf.Reset()

	namespace := "delete"
	expected := strings.Replace(templ, "namespace:", "namespace: "+namespace, -1)
	name := "foo"
	dryrun := true
	ns := &corev1.Namespace{}
	ns.Name = namespace
	_, err := instance.KubernetesClient.CoreV1().Namespaces().Create(ns)
	assert.NoError(t, err)

	createD := &v1.Deployment{}
	err = yaml.Unmarshal([]byte(expected), createD)
	assert.NoError(t, err)
	_, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Create(createD)
	assert.NoError(t, err)

	req := &v1alpha1.ResourceCommand{
		Requests: []v1alpha1.ResourceRequest{
			{
				Version:   "v1",
				Group:     "apps",
				Resource:  "deployments",
				Operation: v1alpha1.DeleteResource,
				BodyTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{index .Flags.Strings "name"}}
  namespace: {{index .Flags.Strings "namespace"}}
`,
			},
		},
	}
	values := parse.Values{
		Flags: parse.Flags{
			Strings: map[string]*string{
				"name":      &name,
				"namespace": &namespace,
			},
			Bools: map[string]*bool{
				"dry-run": &dryrun,
			},
		},
	}

	// Dry Run dispatch
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	d, err := instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, d)

	// Actually dispatch
	dryrun = false
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	d, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.Empty(t, d)
}

// TestDispatchDelete tests that a DeleteResource operation succeeds
func TestDispatchGet(t *testing.T) {
	buf.Reset()

	namespace := "get"
	expected := strings.Replace(templ, "namespace:", "namespace: "+namespace, -1)
	name := "foo"
	dryrun := true
	ns := &corev1.Namespace{}
	ns.Name = namespace
	_, err := instance.KubernetesClient.CoreV1().Namespaces().Create(ns)
	assert.NoError(t, err)

	createD := &v1.Deployment{}
	err = yaml.Unmarshal([]byte(expected), createD)
	assert.NoError(t, err)
	_, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Create(createD)
	assert.NoError(t, err)

	req := &v1alpha1.ResourceCommand{
		Requests: []v1alpha1.ResourceRequest{
			{
				Version:   "v1",
				Group:     "apps",
				Resource:  "deployments",
				Operation: v1alpha1.GetResource,
				BodyTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{index .Flags.Strings "name"}}
  namespace: {{index .Flags.Strings "namespace"}}
  labels:
    app: {{index .Flags.Strings "name"}}
`,
				SaveResponseValues: []v1alpha1.ResponseValue{
					{
						JSONPath: "{.metadata.name}",
						Name:     "name",
					},
					{
						JSONPath: "{.spec.replicas}",
						Name:     "replicas",
					},
				},
			},
		},
	}
	values := parse.Values{
		Flags: parse.Flags{
			Strings: map[string]*string{
				"name":      &name,
				"namespace": &namespace,
			},
			Bools: map[string]*bool{
				"dry-run": &dryrun,
			},
		},
	}

	// Dry Run dispatch
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	d, err := instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, d)
	assert.Empty(t, &values.Responses.Ints)
	assert.Empty(t, &values.Responses.Strings)
	assert.Empty(t, &values.Responses.StringSlices)
	assert.Empty(t, &values.Responses.Bools)
	assert.Empty(t, &values.Responses.Floats)

	// Actually dispatch
	dryrun = false
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	d, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, d)
	assert.NotEmpty(t, &values.Responses.Strings)
	assert.Equal(t, "foo", *values.Responses.Strings["name"])
	assert.Equal(t, "2", *values.Responses.Strings["replicas"])
}

// TestDispatchDelete tests that a DeleteResource operation succeeds
func TestDispatchPrint(t *testing.T) {
	buf.Reset()

	namespace := "print"
	expected := strings.Replace(templ, "namespace:", "namespace: "+namespace, -1)
	name := "foo"
	dryrun := true
	replicas := int32(2)
	image := "nginx"

	req := &v1alpha1.ResourceCommand{
		Requests: []v1alpha1.ResourceRequest{
			{
				Version:   "v1",
				Group:     "apps",
				Resource:  "deployments",
				Operation: v1alpha1.PrintResource,
				BodyTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{index .Flags.Strings "name"}}
  namespace: {{index .Flags.Strings "namespace"}}
  labels:
    app: {{index .Flags.Strings "name"}}
spec:
  replicas: {{index .Flags.Ints "replicas"}}
  selector:
    matchLabels:
      app: {{index .Flags.Strings "name"}}
  template:
    metadata:
      labels:
        app: {{index .Flags.Strings "name"}}
    spec:
      containers:
      - name: {{index .Flags.Strings "name"}}
        image: {{index .Flags.Strings "image"}}
`,
			},
		},
	}
	values := parse.Values{
		Flags: parse.Flags{
			Strings: map[string]*string{
				"name":      &name,
				"namespace": &namespace,
				"image":     &image,
			},
			Ints: map[string]*int32{
				"replicas": &replicas,
			},
			Bools: map[string]*bool{
				"dry-run": &dryrun,
			},
		},
	}

	// Dry Run dispatch
	err := instance.Dispatch(req, &values)
	assert.NoError(t, err)
	assert.Equal(t, expected, buf.String())

	d, err := instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.Empty(t, d)
	assert.Empty(t, &values.Responses.Ints)
	assert.Empty(t, &values.Responses.Strings)
	assert.Empty(t, &values.Responses.StringSlices)
	assert.Empty(t, &values.Responses.Bools)
	assert.Empty(t, &values.Responses.Floats)

	// Actually dispatch
	dryrun = false
	buf.Reset()
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	assert.Equal(t, expected, buf.String())

	d, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.Empty(t, d)
	assert.Empty(t, &values.Responses.Ints)
	assert.Empty(t, &values.Responses.Strings)
	assert.Empty(t, &values.Responses.StringSlices)
	assert.Empty(t, &values.Responses.Bools)
	assert.Empty(t, &values.Responses.Floats)

}

var multiTempl = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: foo
  name: foo
  namespace:
spec:
  replicas: 2
  selector:
    matchLabels:
      app: foo
  template:
    metadata:
      labels:
        app: foo
    spec:
      containers:
      - image: nginx
        name: foo
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: foo
  name: foo
  namespace:
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 9376
  selector:
    app: foo
---
`

// TestDispatchCreate tests that a CreateResource operation succeeds
func TestDispatchMultiCreate(t *testing.T) {
	buf.Reset()

	namespace := "multi"
	expected := strings.Replace(multiTempl, "namespace:", "namespace: "+namespace, -1)
	name := "foo"
	replicas := int32(2)
	image := "nginx"
	dryrun := true
	ns := &corev1.Namespace{}
	ns.Name = namespace
	_, err := instance.KubernetesClient.CoreV1().Namespaces().Create(ns)
	assert.NoError(t, err)

	req := &v1alpha1.ResourceCommand{
		Requests: []v1alpha1.ResourceRequest{
			{
				Version:   "v1",
				Group:     "apps",
				Resource:  "deployments",
				Operation: v1alpha1.CreateResource,
				BodyTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{index .Flags.Strings "name"}}
  namespace: {{index .Flags.Strings "namespace"}}
  labels:
    app: {{index .Flags.Strings "name"}}
spec:
  replicas: {{index .Flags.Ints "replicas"}}
  selector:
    matchLabels:
      app: {{index .Flags.Strings "name"}}
  template:
    metadata:
      labels:
        app: {{index .Flags.Strings "name"}}
    spec:
      containers:
      - name: {{index .Flags.Strings "name"}}
        image: {{index .Flags.Strings "image"}}
`,
				SaveResponseValues: []v1alpha1.ResponseValue{
					{
						JSONPath: "{.metadata.name}",
						Name:     "name",
					},
					{
						JSONPath: "{.spec.replicas}",
						Name:     "replicas",
					},
				},
			},
			{
				Version:   "v1",
				Group:     "",
				Resource:  "services",
				Operation: v1alpha1.CreateResource,
				BodyTemplate: `apiVersion: v1
kind: Service
metadata:
  name: {{ index .Flags.Strings "name" }}
  namespace: {{ index .Flags.Strings "namespace" }}
  labels:
    app: {{ index .Flags.Strings "name" }}
spec:
  selector:
    app: {{ index .Flags.Strings "name" }}
  ports:
  - port: 80
    protocol: TCP
    targetPort: 9376
`,
				SaveResponseValues: []v1alpha1.ResponseValue{
					{
						JSONPath: "{.spec.ports[0].port}",
						Name:     "port",
					},
				},
			},
		},
	}
	values := parse.Values{
		Flags: parse.Flags{
			Strings: map[string]*string{
				"name":      &name,
				"namespace": &namespace,
				"image":     &image,
			},
			Ints: map[string]*int32{
				"replicas": &replicas,
			},
			Bools: map[string]*bool{
				"dry-run": &dryrun,
			},
		},
	}

	// Dry Run dispatch
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)
	assert.Equal(t, expected, buf.String())
	d, err := instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.Empty(t, d)

	// Actually dispatch
	dryrun = false
	err = instance.Dispatch(req, &values)
	assert.NoError(t, err)

	d, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, d)
	assert.NotEmpty(t, &values.Responses.Strings)
	s, err := instance.KubernetesClient.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, s)

	assert.NotEmpty(t, &values.Responses.Strings)
	assert.Equal(t, "foo", *values.Responses.Strings["name"])
	assert.Equal(t, "2", *values.Responses.Strings["replicas"])
	assert.Equal(t, "80", *values.Responses.Strings["port"])
}

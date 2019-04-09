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

package dy_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
	"sigs.k8s.io/cli-experimental/util/dyctl/cmd"
	"sigs.k8s.io/yaml"
)

var instance *dy.CommandBuilder

func TestMain(m *testing.M) {
	var stop func()
	var err error

	instance, stop, err = wiretest.InitializeCommandBuilder(nil)
	if err != nil {
		os.Exit(1)
	}
	defer stop()
	os.Exit(m.Run())
}

func TestBuildCRD(t *testing.T) {
	buf := &bytes.Buffer{}
	instance.Writer.Output = buf

	// Create CRDs with Commands
	namespace := "buildcrd"
	ns := &v1.Namespace{}
	ns.Name = namespace
	_, err := instance.KubernetesClient.CoreV1().Namespaces().Create(ns)
	assert.NoError(t, err)

	// First CRD
	crd1 := &v1beta1.CustomResourceDefinition{}
	err = yaml.UnmarshalStrict([]byte(crd1YAML), crd1)
	assert.NoError(t, err)
	cl1 := &v1alpha1.ResourceCommandList{}
	err = yaml.UnmarshalStrict([]byte(commandListYAML1), cl1)
	assert.NoError(t, err)
	err = cmd.AddTo(cl1, crd1)
	assert.NoError(t, err)
	_, err = instance.Lister.Client.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd1)
	assert.NoError(t, err)

	// Second CRD
	crd2 := &v1beta1.CustomResourceDefinition{}
	err = yaml.UnmarshalStrict([]byte(crd2YAML), crd2)
	assert.NoError(t, err)
	cl2 := &v1alpha1.ResourceCommandList{}
	err = yaml.UnmarshalStrict([]byte(commandListYAML2), cl2)
	assert.NoError(t, err)
	err = cmd.AddTo(cl2, crd2)
	assert.NoError(t, err)
	_, err = instance.Lister.Client.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd2)
	assert.NoError(t, err)

	// Build Command for dry-run test
	buf.Reset()
	root := &cobra.Command{}
	err = instance.Build(root, nil)
	assert.NoError(t, err)

	// Verify Command structure
	assert.Len(t, root.Commands(), 1)
	assert.Len(t, root.Commands()[0].Commands(), 2)
	cmd1, _, err := root.Find([]string{"create", "deployment"})
	assert.NoError(t, err)
	assert.Equal(t, "deployment", cmd1.Use)
	assert.Equal(t, `# Create a new deployment named my-dep that runs the busybox image.
kubectl create deployment --name my-dep --image=busybox
`, cmd1.Example)
	assert.Equal(t, `Create a deployment with the specified name - long.`, cmd1.Long)
	assert.Equal(t, `Create a deployment with the specified name - short.`, cmd1.Short)
	assert.Equal(t, []string{"deploy", "deployments"}, cmd1.Aliases)

	// Set namespace because it will be set by the common cli flags rather than on the command itself
	cmd1.Flags().String("namespace", "", "")

	cmd2, _, err := root.Find([]string{"create", "service"})
	assert.NoError(t, err)
	assert.Equal(t, "service", cmd2.Use)

	// Verify dry-run behavior
	root.SetArgs([]string{
		"create", "deployment", "--name=foo", "--namespace=" + namespace, "--image=nginx", "--dry-run"})
	err = root.Execute()
	assert.NoError(t, err)
	assert.Equal(t, "foo", cmd1.Flag("name").Value.String())
	assert.Equal(t, namespace, cmd1.Flag("namespace").Value.String())
	assert.Equal(t, "nginx", cmd1.Flag("image").Value.String())
	assert.Equal(t, "true", cmd1.Flag("dry-run").Value.String())
	assert.Equal(t, expectedString, buf.String())

	// Check Deployment was NOT created
	d, err := instance.KubernetesClient.AppsV1().Deployments(namespace).Get("foo", metav1.GetOptions{})
	assert.Error(t, err)
	assert.Empty(t, d)

	// Build Command for non-dry-run test
	buf.Reset()
	root = &cobra.Command{}
	err = instance.Build(root, nil)
	assert.NoError(t, err)
	cmd1, _, err = root.Find([]string{"create", "deployment"})
	assert.NoError(t, err)
	// Set namespace because it will be set by the common cli flags rather than on the command itself
	cmd1.Flags().String("namespace", "", "")

	// Verify non-dry-run behavior
	root.SetArgs([]string{
		"create", "deployment", "--name=foo", "--namespace=" + namespace, "--image=nginx"})
	err = root.Execute()
	assert.NoError(t, err)

	// Check Deployment was created
	d, err = instance.KubernetesClient.AppsV1().Deployments(namespace).Get("foo", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, d)

	// Check output
}

var expectedString = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: foo
  name: foo
  namespace: buildcrd
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

var crd1YAML = `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: "clitestresourcesa.test.cli.sigs.k8s.io"
spec:
  group: test.cli.sigs.k8s.io
  names:
    kind: CLITestResource
    plural: clitestresourcesa
  scope: Namespaced
  version: v1alpha1
---
`

var crd2YAML = `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: "clitestresourcesb.test.cli.sigs.k8s.io"
spec:
  group: test.cli.sigs.k8s.io
  names:
    kind: CLITestResource
    plural: clitestresourcesb
  scope: Namespaced
  version: v1alpha1
---
`

var commandListYAML1 = `items:
- command:
    path:
    - "create" # Command is a subcommand of this path
    use: "deployment" # Command use
    aliases: # Command alias'
    - "deploy"
    - "deployments"
    short: Create a deployment with the specified name - short.
    long: Create a deployment with the specified name - long.
    example: |
        # Create a new deployment named my-dep that runs the busybox image.
        kubectl create deployment --name my-dep --image=busybox
    flags:
    - name: name
      type: String
      stringValue: ""
      description: deployment name
    - name: image
      type: String
      stringValue: ""
      description: Image name to run.
    - name: replicas
      type: Int
      intValue: 2
      description: Image name to run.
  requests:
  - group: apps
    version: v1
    resource: deployments
    operation: Create
    bodyTemplate: |
      apiVersion: apps/v1
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
    saveResponseValues:
    - name: responsename
      jsonPath: "{.metadata.name}"
  output: |
    deployment.apps/{{index .Responses.Strings "responsename"}} created
`

var commandListYAML2 = `items:
- command:
    path:
    - "create" # Command is a subcommand of this path
    use: "service" # Command use
    short: Create a service with the specified name.
    long: Create a service with the specified name.
    example: |
        # Create a new service
        kubectl create service --name my-dep --label-key app --label-value foo
    flags:
    - name: name
      type: String
      stringValue: ""
      description: service name
    - name: label-key
      type: String
      stringValue: ""
      description: label key to select.
    - name: label-value
      type: String
      stringValue: ""
      description: label value to select.
  requests:
  - group: ""
    version: v1
    resource: services
    operation: Create
    bodyTemplate: |
      apiVersion: v1
      kind: Service
      metadata:
        name: {{index .Flags.Strings "name"}}
        namespace: {{index .Flags.Strings "namespace"}}
        labels:
          {{ index .Flags.Strings "label-key"}}: {{index .Flags.Strings "label-value"}}
      spec:
        selector:
          {{ index .Flags.Strings "label-key"}}: {{index .Flags.Strings "label-value"}}
  output: |
    service/{{index .Flags.Strings "name"}} created
`

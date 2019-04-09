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

package cmd_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"sigs.k8s.io/cli-experimental/cmd"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	"github.com/stretchr/testify/assert"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/cli-experimental/internal/pkg/wirecli/wiretest"
	dycmd "sigs.k8s.io/cli-experimental/util/dyctl/cmd"
	"sigs.k8s.io/yaml"
)

var cfg *rest.Config

func TestMain(m *testing.M) {
	c, stop, err := wiretest.NewRestConfig()
	cfg = c
	if err != nil {
		os.Exit(1)
	}
	defer stop()
	os.Exit(m.Run())
}

func TestAddDyCommands(t *testing.T) {
	cs, err := clientset.NewForConfig(cfg)
	assert.NoError(t, err)

	// Add the CRD
	crd1 := &v1beta1.CustomResourceDefinition{}
	err = yaml.UnmarshalStrict([]byte(crd1YAML), crd1)
	assert.NoError(t, err)
	cl1 := &v1alpha1.ResourceCommandList{}
	err = yaml.UnmarshalStrict([]byte(commandListYAML1), cl1)
	assert.NoError(t, err)
	err = dycmd.AddTo(cl1, crd1)
	assert.NoError(t, err)
	_, err = cs.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd1)
	assert.NoError(t, err)

	// Create the Command Argumnents
	buf := &bytes.Buffer{}
	args := []string{
		"create", "deployment", "--server=" + cfg.Host, "--name=foo", "--namespace=cmd",
		"--image=nginx", "--dry-run"}

	// Set the Output and Arguments
	fn := func(c *cobra.Command) {
		c.SetArgs(args)
		c.SetOutput(buf)
	}

	// Execute the Command
	err = cmd.Execute(args, fn)
	assert.NoError(t, err)

	// Verify the dry-run output
	assert.Equal(t, expectedString, buf.String())
}

var expectedString = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: foo
  name: foo
  namespace: cmd
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

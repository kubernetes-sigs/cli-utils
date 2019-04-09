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

package cmd

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/yaml"
)

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add-commands",
	Short: "Add a Dynamic Command to a CRD Resource.",
	Long: `Add a Dynamic Command to a CRD Resource.

Reads a yaml file containing a ResourceCommandList updates a CRD Resource Config yaml file
to publish the Commands.
`,
	Example: `# Add the ResourceCommandList from commands.yaml to the CRD definition in crd.yaml
dy add-commands commands.yaml crd.yaml
`,
	RunE: runE,
	Args: cobra.ExactArgs(2),
}

func runE(cmd *cobra.Command, args []string) error {

	// Parse ResourceCommandList
	clBytes, err := ioutil.ReadFile(args[0])
	if err != nil {
		return err
	}
	commandList := v1alpha1.ResourceCommandList{}
	if err := yaml.Unmarshal(clBytes, &commandList); err != nil {
		return err
	}

	// Parse CustomResourceDefinition
	crd := unstructured.Unstructured{}
	crdBytes, err := ioutil.ReadFile(args[1])
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(crdBytes, &crd); err != nil {
		return err
	}

	crd.SetAPIVersion("apiextensions.k8s.io/v1beta1")
	crd.SetKind("CustomResourceDefinition")
	if err := AddTo(&commandList, &crd); err != nil {
		return err
	}

	// Write the CRD output
	crdOut, err := yaml.Marshal(crd.Object)
	if err != nil {
		return err
	}

	info, err := os.Stat(args[1])
	if err != nil {
		return err
	}
	return ioutil.WriteFile(args[1], crdOut, info.Mode())
}

// AddTo adds the commandList as an annotation and label to the crd
func AddTo(commandList *v1alpha1.ResourceCommandList, crd v1.Object) error {
	// Add the Label
	lab := crd.GetLabels()
	if lab == nil {
		lab = map[string]string{}
	}
	lab["cli-experimental.sigs.k8s.io/ResourceCommandList"] = ""
	crd.SetLabels(lab)

	// Add the Annotation
	clOut, err := yaml.Marshal(commandList)
	if err != nil {
		return err
	}
	clOutJSON, err := yaml.YAMLToJSONStrict(clOut)
	if err != nil {
		return err
	}
	clAnn := &bytes.Buffer{}
	err = json.Compact(clAnn, clOutJSON)
	if err != nil {
		return err
	}

	an := crd.GetAnnotations()
	if an == nil {
		an = map[string]string{}
	}
	an["cli-experimental.sigs.k8s.io/ResourceCommandList"] = string(clAnn.String())
	crd.SetAnnotations(an)
	return nil
}

func init() {
	rootCmd.AddCommand(addCmd)
}

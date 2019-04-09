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

package list

import (
	"fmt"
	"os"
	"reflect"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clidynamic "sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/yaml"
)

const (
	annotation = "cli-experimental.sigs.k8s.io/ResourceCommandList"
	label      = annotation
)

// CommandLister lists commands from CRDs
type CommandLister struct {
	// Client talks to an apiserver to list CRDs
	Client *clientset.Clientset

	// DynamicClient is used to make requests
	DynamicClient dynamic.Interface
}

var gvk = schema.GroupVersionResource{
	Resource: "resourcecommands", Group: "dynamic.cli.sigs.k8s.io", Version: "v1alpha1"}

// List fetches the list of dynamic commands published as Annotations on CRDs
func (cl *CommandLister) List(options *metav1.ListOptions) (clidynamic.ResourceCommandList, error) {
	client := cl.Client.ApiextensionsV1beta1().CustomResourceDefinitions()
	cmds := clidynamic.ResourceCommandList{}

	if options == nil {
		options = &metav1.ListOptions{LabelSelector: label}
	}

	// Get ResourceCommand CRs if they exist
	rcs, err := cl.DynamicClient.Resource(gvk).List(*options)
	if err == nil {
		// Ignore errors because the CRD for ResourceCommands may not be defined
		for i := range rcs.Items {
			rc := rcs.Items[i]
			rcBytes, err := yaml.Marshal(rc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to Marshal ResourceCommand %s: %v\n", rc.GetName(), err)
				continue
			}
			cmd := clidynamic.ResourceCommand{}
			err = yaml.UnmarshalStrict(rcBytes, &cmd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to Unmarshal ResourceCommand %s: %v\n", rc.GetName(), err)
				continue
			}
			cmds.Items = append(cmds.Items, cmd)
		}
	}

	// Get CRDs with ResourceCommands as annotations
	crds, err := client.List(*options)
	if err != nil {
		return cmds, err
	}

	for i := range crds.Items {
		crd := crds.Items[i]
		// Get the ResourceCommand json
		s := crd.Annotations[annotation]
		if len(s) == 0 {
			fmt.Fprintf(os.Stderr, "CRD missing ResourceCommand annotation %s: %v\n", crd.Name, err)
			continue
		}

		// Unmarshall the annotation value into a ResourceCommandList
		rcList := clidynamic.ResourceCommandList{}
		if err := yaml.UnmarshalStrict([]byte(s), &rcList); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse commands for CRD %s: %v\n", crd.Name, err)
			continue
		}

		// Verify we parsed something
		if reflect.DeepEqual(rcList, clidynamic.ResourceCommandList{}) {
			fmt.Fprintf(os.Stderr, "no commands for CRD %s: %s\n", crd.Name, s)
			continue
		}

		// Add the commands to the list
		for i := range rcList.Items {
			item := rcList.Items[i]
			if len(item.Requests) > 0 {
				cmds.Items = append(cmds.Items, item)
			}
		}
	}
	return cmds, nil
}

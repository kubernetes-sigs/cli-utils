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

package dispatch

import (
	"bytes"
	"fmt"
	"html/template"

	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/cli-experimental/internal/pkg/apis/dynamic/v1alpha1"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/output"
	"sigs.k8s.io/cli-experimental/internal/pkg/dy/parse"
	"sigs.k8s.io/yaml"
)

// Dispatcher dispatches requests for a Command
type Dispatcher struct {
	// KubernetesClient is used to make requests
	KubernetesClient *kubernetes.Clientset

	// DynamicClient is used to make requests
	DynamicClient dynamic.Interface

	// Writer writes templatized output
	Writer *output.CommandOutputWriter
}

// Dispatch sends requests to the apiserver for the Command.
func (d *Dispatcher) Dispatch(cmd *v1alpha1.ResourceCommand, values *parse.Values) error {
	// Iterate over requests
	for i := range cmd.Requests {
		req := cmd.Requests[i]
		if err := d.do(req, cmd.Command.Use, values); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) do(req v1alpha1.ResourceRequest, name string, values *parse.Values) error {
	// Build the request object
	obj, err := templateToResource(req.BodyTemplate, name+"-resource-request", values)
	if err != nil {
		return err
	}

	// Check if it is dry-do
	if values.IsDryRun() {
		// Simply print the object rather than making the request
		o, err := yaml.Marshal(obj.Object)
		if err != nil {
			return fmt.Errorf("%v\n%+v", err, obj.Object)
		}
		fmt.Fprintf(d.Writer.Output, "%s---\n", string(o))

		// Skip making requests for dry-do
		return nil
	}

	// Send the request
	gvr := schema.GroupVersionResource{Resource: req.Resource, Version: req.Version, Group: req.Group}
	resp, err := d.doRequest(obj, gvr, req.Operation)
	if err != nil {
		return err
	}

	// Save the response values so they can be used when writing the output
	return doResponse(req, resp, values)

}

// doRequest makes a request to the apiserver with the object (request body), operation (request http operation),
// group,version,resource (request url).
func (d *Dispatcher) doRequest(
	obj *unstructured.Unstructured,
	gvr schema.GroupVersionResource,
	op v1alpha1.ResourceOperation) (*unstructured.Unstructured, error) {

	req := d.DynamicClient.Resource(gvr).Namespace(obj.GetNamespace())
	var resp = &unstructured.Unstructured{}
	var o []byte
	var err error

	switch op {
	case v1alpha1.PrintResource:
		// Only print the object
		if o, err = yaml.Marshal(obj.Object); err == nil {
			fmt.Fprintf(d.Writer.Output, "%s---\n", string(o))
		}
	case v1alpha1.CreateResource:
		resp, err = req.Create(obj, metav1.CreateOptions{})
	case v1alpha1.DeleteResource:
		err = req.Delete(obj.GetName(), &metav1.DeleteOptions{})
	case v1alpha1.UpdateResource:
		resp, err = req.Update(obj, metav1.UpdateOptions{})
	case v1alpha1.GetResource:
		resp, err = req.Get(obj.GetName(), metav1.GetOptions{})
	case v1alpha1.PatchResource:
	}
	return resp, err
}

// templateToResource builds an Unstructured object from a template and flag values
func templateToResource(t, name string, values *parse.Values) (*unstructured.Unstructured, error) {
	temp, err := template.New(name).Parse(t)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, t)
	}

	body := &bytes.Buffer{}
	if err := temp.Execute(body, values); err != nil {
		return nil, fmt.Errorf("%v\n%s\n%v", err, t, values)
	}

	// Parse the Resource into an Unstructured object
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if err := yaml.Unmarshal(body.Bytes(), &obj.Object); err != nil {
		return nil, fmt.Errorf("%v\n%s", err, body.String())
	}

	return obj, nil
}

// doResponse parses the items specified by JSONPath from the response object back into the Flags struct
// so that the response is available in the output template
func doResponse(req v1alpha1.ResourceRequest, resp *unstructured.Unstructured, res *parse.Values) error {
	if res.Responses.Strings == nil {
		res.Responses.Strings = map[string]*string{}
	}
	for i := range req.SaveResponseValues {
		v := req.SaveResponseValues[i]
		j := jsonpath.New(v.Name)
		buf := &bytes.Buffer{}
		if err := j.Parse(v.JSONPath); err != nil {
			return err
		}
		if err := j.Execute(buf, resp.Object); err != nil {
			return err
		}
		s := buf.String()
		res.Responses.Strings[v.Name] = &s
	}
	return nil
}

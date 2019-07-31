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

package v1alpha1

// ResourceCommandList contains a list of Commands
type ResourceCommandList struct {
	Items []ResourceCommand `json:"items"`
}

// ResourceCommand defines a command that is dynamically defined as an annotation on a CRD
type ResourceCommand struct {
	// Command is the cli Command
	Command Command `json:"command"`

	// Requests are the requests the command will send to the apiserver.
	// +optional
	Requests []ResourceRequest `json:"requests,omitempty"`

	// Output is a go-template used write the command output.  It may reference values specified as flags using
	// {{index .Flags.Strings "flag-name"}}, {{index .Flags.Ints "flag-name"}}, {{index .Flags.Bools "flag-name"}},
	// {{index .Flags.Floats "flag-name"}}.
	//
	// It may also reference values from the responses that were saved using saveResponseValues
	// - {{index .Responses.Strings "response-value-name"}}.
	//
	// Example:
	// 		deployment.apps/{{index .Responses.Strings "responsename"}} created
	//
	// +optional
	Output string `json:"output,omitempty"`
}

// ResourceOperation specifies the type of Request operation
type ResourceOperation string

const (
	// CreateResource performs a Create Request
	CreateResource ResourceOperation = "Create"
	// UpdateResource performs an Update Request
	UpdateResource = "Update"
	// DeleteResource performs a Delete Request
	DeleteResource = "Delete"
	// GetResource performs a Get Request
	GetResource = "Get"
	// PatchResource performs a Patch Request
	PatchResource = "Patch"
	// PrintResource prints the Resource
	PrintResource = "Print"
)

// ResourceRequest defines a request made by the cli to the apiserver
type ResourceRequest struct {
	// Group is the API group of the request endpoint
	//
	// Example: apps
	Group string `json:"group"`

	// Version is the API version of the request endpoint
	//
	// Example: v1
	Version string `json:"version"`

	// Resource is the API resource of the request endpoint
	//
	// Example: deployments
	Resource string `json:"resource"`

	// Operation is the type of operation to perform for the request.  One of: Create, Update, Delete, Get, Patch
	Operation ResourceOperation `json:"operation"`

	// BodyTemplate is a go-template for the request Body.  It may reference values specified as flags using
	// {{index .Flags.Strings "flag-name"}}, {{index .Flags.Ints "flag-name"}}, {{index .Flags.Bools "flag-name"}},
	// {{index .Flags.Floats "flag-name"}}
	//
	// Example:
	//      apiVersion: apps/v1
	//      kind: Deployment
	//      metadata:
	//        name: {{index .Flags.Strings "name"}}
	//        namespace: {{index .Flags.Strings "namespace"}}
	//        labels:
	//          app: nginx
	//      spec:
	//        replicas: {{index .Flags.Ints "replicas"}}
	//        selector:
	//          matchLabels:
	//            app: {{index .Flags.Strings "name"}}
	//        template:
	//          metadata:
	//            labels:
	//              app: {{index .Flags.Strings "name"}}
	//          spec:
	//            containers:
	//            - name: {{index .Flags.Strings "name"}}
	//              image: {{index .Flags.Strings "image"}}
	//
	// +optional
	BodyTemplate string `json:"bodyTemplate,omitempty"`

	// SaveResponseValues are values read from the response and saved in {{index .Responses.Strings "flag-name"}}.
	// They may be used in the ResourceCommand.Output go-template.
	//
	// Example:
	//		- name: responsename
	//        jsonPath: "{.metadata.name}"
	//
	// +optional
	SaveResponseValues []ResponseValue `json:"saveResponseValues,omitempty"`
}

// Flag defines a cli flag that should be registered and available in request / output templates
type Flag struct {
	Type FlagType `json:"type"`

	Name string `json:"name"`

	Description string `json:"description"`

	// +optional
	Required *bool `json:"required,omitempty"`

	// +optional
	StringValue string `json:"stringValue,omitempty"`

	// +optional
	StringSliceValue []string `json:"stringSliceValue,omitempty"`

	// +optional
	BoolValue bool `json:"boolValue,omitempty"`

	// +optional
	IntValue int32 `json:"intValue,omitempty"`

	// +optional
	FloatValue float64 `json:"floatValue,omitempty"`
}

// ResponseValue defines a value that should be parsed from a response and available in output templates
type ResponseValue struct {
	Name     string `json:"name"`
	JSONPath string `json:"jsonPath"`
}

// FlagType defines the type of flag to register in the cli
type FlagType string

const (
	// String defines a string flag
	String FlagType = "String"
	// Bool defines a bool flag
	Bool = "Bool"
	// Float defines a float flag
	Float = "Float"
	// Int defines an int flag
	Int = "Int"
	// StringSlice defines a string slice flag
	StringSlice = "StringSlice"
)

// Command defines a Command published on a CRD and created as a cobra Command in the cli
type Command struct {
	// Use is the one-line usage message.
	Use string `json:"use"`

	// Path is the path to the sub-command.  Omit if the command is directly under the root command.
	// +optional
	Path []string `json:"path,omitempty"`

	// Short is the short description shown in the 'help' output.
	// +optional
	Short string `json:"short,omitempty"`

	// Long is the long message shown in the 'help <this-command>' output.
	// +optional
	Long string `json:"long,omitempty"`

	// Example is examples of how to use the command.
	// +optional
	Example string `json:"example,omitempty"`

	// Deprecated defines, if this command is deprecated and should print this string when used.
	// +optional
	Deprecated string `json:"deprecated,omitempty"`

	// Flags are the command line flags.
	//
	// Example:
	// 		  - name: namespace
	//    		type: String
	//    		stringValue: "default"
	//    		description: "deployment namespace"
	//
	// +optional60
	Flags []Flag `json:"flags,omitempty"`

	// SuggestFor is an array of command names for which this command will be suggested -
	// similar to aliases but only suggests.
	SuggestFor []string `json:"suggestFor,omitempty"`

	// Aliases is an array of aliases that can be used instead of the first word in Use.
	Aliases []string `json:"aliases,omitempty"`

	// Version defines the version for this command. If this value is non-empty and the command does not
	// define a "version" flag, a "version" boolean flag will be added to the command and, if specified,
	// will print content of the "Version" variable.
	// +optional
	Version string `json:"version,omitempty"`
}

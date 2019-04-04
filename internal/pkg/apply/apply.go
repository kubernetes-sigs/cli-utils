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

package apply

import (
	"fmt"
	"io"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
)

// Apply applies directories
type Apply struct {
	Clientset *kubernetes.Clientset
	Out       io.Writer
	Resources clik8s.ResourceConfigs
	Commit    *object.Commit
}

// Result contains the Apply Result
type Result struct {
	Resources clik8s.ResourceConfigs
}

// Do executes the apply
func (a *Apply) Do() (Result, error) {
	fmt.Fprintf(a.Out, "Doing `cli-experimental apply`\n")
	pods, err := a.Clientset.CoreV1().Pods("default").List(metav1.ListOptions{})
	if err != nil {
		return Result{}, err
	}
	for _, p := range pods.Items {
		fmt.Fprintf(a.Out, "Pod %s\n", p.Name)
	}

	return Result{Resources: a.Resources}, nil
}

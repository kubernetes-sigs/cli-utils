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

package status

import (
	"fmt"
	"io"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
)

// Status returns the status for rollouts
type Status struct {
	Resources clik8s.ResourceConfigs
	Out       io.Writer
	Clientset *kubernetes.Clientset
	Commit    *object.Commit
}

// Result contains the Status Result
type Result struct {
	Resources clik8s.ResourceConfigs
}

// Do executes the apply
func (s *Status) Do() (Result, error) {
	fmt.Fprintf(s.Out, "Doing `cli-experimental apply status`\n")
	if s.Commit != nil {
		fmt.Fprintf(s.Out, "Commit %s\n", s.Commit.Hash.String())
	}
	pods, err := s.Clientset.CoreV1().Pods("default").List(metav1.ListOptions{})
	if err != nil {
		return Result{}, err
	}
	for _, p := range pods.Items {
		fmt.Fprintf(s.Out, "Pod %s\n", p.Name)
	}

	return Result{Resources: s.Resources}, nil
}

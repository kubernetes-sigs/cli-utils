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

package wirek8s

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/wire"
	"github.com/spf13/pflag"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/kustomize"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/yaml"

	// for connecting to various types of hosted clusters
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// ProviderSet defines dependencies for initializing Kubernetes objects
var ProviderSet = wire.NewSet(NewKubernetesClientSet, NewExtensionsClientSet, NewConfigFlags, NewRestConfig,
	NewResourceConfig, NewFileSystem, NewDynamicClient)

// Flags registers flags for talkig to a Kubernetes cluster
func Flags(fs *pflag.FlagSet) {
	kubeConfigFlags := genericclioptions.NewConfigFlags(false)
	kubeConfigFlags.AddFlags(fs)
}

// HelpFlags is a list of flags to strips
var HelpFlags = []string{"-h", "--help"}

// WordSepNormalizeFunc normalizes flags
func WordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.Replace(name, "_", "-", -1))
	}
	return pflag.NormalizedName(name)
}

// NewConfigFlags parses flags used to generate the *rest.Config
func NewConfigFlags(ar util.Args) (*genericclioptions.ConfigFlags, error) {
	a := CopyStrSlice([]string(ar))

	// IMPORTANT: If there is an error parsing flags--continue.
	kubeConfigFlagSet := pflag.NewFlagSet("dispatcher-kube-config", pflag.ContinueOnError)
	kubeConfigFlagSet.ParseErrorsWhitelist.UnknownFlags = true
	kubeConfigFlagSet.SetNormalizeFunc(WordSepNormalizeFunc)
	kubeConfigFlagSet.Set("namespace", "default")

	unusedParameter := true // Could be either true or false
	kubeConfigFlags := genericclioptions.NewConfigFlags(unusedParameter)
	kubeConfigFlags.AddFlags(kubeConfigFlagSet)

	// Remove help flags, since these are special-cased in pflag.Parse,
	args := FilterList(a, HelpFlags)
	if err := kubeConfigFlagSet.Parse(args); err != nil {
		return nil, err
	}

	return kubeConfigFlags, nil
}

// FilterList returns a copy of "l" with elements from "toRemove" filtered out.
func FilterList(l []string, rl []string) []string {
	c := CopyStrSlice(l)
	for _, r := range rl {
		c = RemoveAllElements(c, r)
	}
	return c
}

// RemoveAllElements removes all elements from "s" which match the string "r".
func RemoveAllElements(s []string, r string) []string {
	for i, rlen := 0, len(s); i < rlen; i++ {
		j := i - (rlen - len(s))
		if s[j] == r {
			s = append(s[:j], s[j+1:]...)
		}
	}
	return s
}

// CopyStrSlice returns a copy of the slice of strings.
func CopyStrSlice(s []string) []string {
	c := make([]string, len(s))
	copy(c, s)
	return c
}

// NewRestConfig returns a new rest.Config parsed from --kubeconfig and --master
func NewRestConfig(f *genericclioptions.ConfigFlags) (*rest.Config, error) {
	return f.ToRESTConfig()
}

// NewKubernetesClientSet provides a clientset for talking to k8s clusters
func NewKubernetesClientSet(c *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(c)
}

// NewExtensionsClientSet provides an apiextensions ClientSet
func NewExtensionsClientSet(c *rest.Config) (*clientset.Clientset, error) {
	return clientset.NewForConfig(c)
}

// NewDynamicClient provides a dynamic.Interface
func NewDynamicClient(c *rest.Config) (dynamic.Interface, error) {
	return dynamic.NewForConfig(c)
}

// NewFileSystem provides a real FileSystem
func NewFileSystem() fs.FileSystem {
	return fs.MakeRealFS()
}

// NewResourceConfig provides ResourceConfigs read from the ResourceConfigPath and FileSystem.
func NewResourceConfig(rcp clik8s.ResourceConfigPath, sysFs fs.FileSystem) (clik8s.ResourceConfigs, error) {
	p := string(rcp)
	var values clik8s.ResourceConfigs

	// TODO: Support urls
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}

	// Kustomization file.  Don't allow recursing on directories with raw Resource Config,
	// should use a kustomization.yaml instead.
	if fi.IsDir() {
		k, err := doDir(p, sysFs)
		if err != nil {
			return nil, err
		}
		values = append(values, k...)
		return values, nil
	}

	r, err := doFile(p)
	if err != nil {
		return nil, err
	}
	values = append(values, r...)

	return values, nil
}

func doDir(p string, sysFs fs.FileSystem) (clik8s.ResourceConfigs, error) {
	var values clik8s.ResourceConfigs
	buf := &bytes.Buffer{}
	err := kustomize.RunKustomizeBuild(buf, sysFs, p)
	if err != nil {
		return nil, err
	}
	objs := strings.Split(buf.String(), "---")
	for _, o := range objs {
		body := map[string]interface{}{}

		if err := yaml.Unmarshal([]byte(o), &body); err != nil {
			return nil, err
		}
		values = append(values, &unstructured.Unstructured{Object: body})
	}
	return values, nil
}

func doFile(p string) (clik8s.ResourceConfigs, error) {
	var values clik8s.ResourceConfigs

	// Don't allow running on kustomization.yaml, prevents weird things like globbing
	if filepath.Base(p) == "kustomization.yaml" {
		return nil, fmt.Errorf(
			"cannot run on kustomization.yaml - use the directory (%v) instead", filepath.Dir(p))
	}

	// Resource file
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	objs := strings.Split(string(b), "---")
	for _, o := range objs {
		body := map[string]interface{}{}

		if err := yaml.Unmarshal([]byte(o), &body); err != nil {
			return nil, err
		}
		values = append(values, &unstructured.Unstructured{Object: body})
	}

	return values, nil
}

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
	"strings"

	"github.com/google/wire"
	"github.com/spf13/pflag"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/configflags"
	"sigs.k8s.io/cli-experimental/internal/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	// for connecting to various types of hosted clusters
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// ProviderSet defines dependencies for initializing Kubernetes objects
var ProviderSet = wire.NewSet(
	NewKubernetesClientSet,
	NewExtensionsClientSet,
	NewConfigFlags,
	NewRestConfig,
	NewDynamicClient,
	NewRestMapper,
	NewClient,
)

// Flags registers flags for talkig to a Kubernetes cluster
func Flags(fs *pflag.FlagSet) {
	kubeConfigFlags := configflags.NewConfigFlags(false)
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
func NewConfigFlags(ar util.Args) (*configflags.ConfigFlags, error) {
	a := CopyStrSlice([]string(ar))

	// IMPORTANT: If there is an error parsing flags--continue.
	kubeConfigFlagSet := pflag.NewFlagSet("dispatcher-kube-config", pflag.ContinueOnError)
	kubeConfigFlagSet.ParseErrorsWhitelist.UnknownFlags = true
	kubeConfigFlagSet.SetNormalizeFunc(WordSepNormalizeFunc)
	kubeConfigFlagSet.Set("namespace", "default")

	unusedParameter := true // Could be either true or false
	kubeConfigFlags := configflags.NewConfigFlags(unusedParameter)
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
func NewRestConfig(f *configflags.ConfigFlags) (*rest.Config, error) {
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

// NewDynamicClient returns a Dynamic Client
func NewDynamicClient(c *rest.Config) (dynamic.Interface, error) {
	return dynamic.NewForConfig(c)
}

// NewRestMapper provides a Discovery rest mapper
func NewRestMapper(c *rest.Config) (meta.RESTMapper, error) {
	return apiutil.NewDiscoveryRESTMapper(c)
}

// NewClient provides a dynamic.Interface
func NewClient(d dynamic.Interface, m meta.RESTMapper) (client.Client, error) {
	return client.NewForConfig(d, m)
}

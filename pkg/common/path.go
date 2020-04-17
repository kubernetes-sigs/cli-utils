// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/apply/prune"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/kioutil"
)

func processPaths(paths []string) genericclioptions.FileNameFlags {
	// No arguments means we are reading from StdIn
	fileNameFlags := genericclioptions.FileNameFlags{}
	if len(paths) == 0 {
		fileNames := []string{"-"}
		fileNameFlags.Filenames = &fileNames
		return fileNameFlags
	}

	t := true
	fileNameFlags.Filenames = &paths
	fileNameFlags.Recursive = &t
	return fileNameFlags
}

func DemandOneDirectory(paths []string) (genericclioptions.FileNameFlags, error) {
	result := processPaths(paths)
	// alas, the things called file names should have been called paths.
	if len(*result.Filenames) != 1 {
		return result, fmt.Errorf(
			"specify exactly one directory path argument; rejecting %v", paths)
	}
	path := (*result.Filenames)[0]
	if !isPathADirectory(path) {
		return result, fmt.Errorf("argument '%s' is not but must be a directory", path)
	}
	return result, nil
}

func isPathADirectory(name string) bool {
	if fi, err := os.Stat(name); err == nil {
		if fi.Mode().IsDir() {
			return true
		}
	}
	return false
}

// ExpandPackageDir expands the one package directory entry in the flags to all
// the config file paths recursively. Excludes the inventory object (since
// this object is specially processed). Used for the diff command, so it will
// not always show a diff of the inventory object. Must be called AFTER
// DemandOneDirectory.
func ExpandPackageDir(f genericclioptions.FileNameFlags) (genericclioptions.FileNameFlags, error) {
	if len(*f.Filenames) != 1 {
		return f, fmt.Errorf("expand package directory should pass one package directory. "+
			"Passed the following paths: %v", f.Filenames)
	}
	configFilepaths, err := expandDir((*f.Filenames)[0])
	if err != nil {
		return f, err
	}
	f.Filenames = &configFilepaths
	return f, nil
}

// expandDir takes a single package directory as a parameter, and returns
// an array of config file paths excluding the inventory object. Returns
// an error if one occurred while processing the paths.
func expandDir(dir string) ([]string, error) {
	filepaths := []string{}
	r := kio.LocalPackageReader{PackagePath: dir}
	nodes, err := r.Read()
	if err != nil {
		return filepaths, err
	}
	for _, node := range nodes {
		meta, err := node.GetMeta()
		if err != nil {
			continue
		}
		// If object has inventory label, skip it.
		labels := meta.Labels
		if _, exists := labels[prune.GroupingLabel]; exists {
			continue
		}
		path := meta.Annotations[kioutil.PathAnnotation]
		path = filepath.Join(dir, path)
		filepaths = append(filepaths, path)
	}
	return filepaths, nil
}

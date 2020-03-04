// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"fmt"
	"os"

	"k8s.io/cli-runtime/pkg/genericclioptions"
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

func demandOneDirectory(paths []string) (genericclioptions.FileNameFlags, error) {
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

package apply

import (
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

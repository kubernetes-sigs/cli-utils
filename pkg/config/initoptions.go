// Copyright 2020 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

const (
	manifestFilename = "inventory-template.yaml"
)
const configMapTemplate = `# NOTE: auto-generated. Some fields should NOT be modified.
# Date: <DATETIME>
#
# Contains the "inventory object" template ConfigMap.
# When this object is applied, it is handled specially,
# storing the metadata of all the other objects applied.
# This object and its stored inventory is subsequently
# used to calculate the set of objects to automatically
# delete (prune), when an object is omitted from further
# applies. When applied, this "inventory object" is also
# used to identify the entire set of objects to delete.
#
# NOTE: The name of this inventory template file
# (e.g. ` + manifestFilename + `) does NOT have any
# impact on group-related functionality such as deletion
# or pruning.
#
apiVersion: v1
kind: ConfigMap
metadata:
  # DANGER: Do not change the inventory object namespace.
  # Changing the namespace will cause a loss of continuity
  # with previously applied grouped objects. Set deletion
  # and pruning functionality will be impaired.
  namespace: <NAMESPACE>
  # NOTE: The name of the inventory object does NOT have
  # any impact on group-related functionality such as
  # deletion or pruning.
  name: inventory-<RANDOMSUFFIX>
  labels:
    # DANGER: Do not change the value of this label.
    # Changing this value will cause a loss of continuity
    # with previously applied grouped objects. Set deletion
    # and pruning functionality will be impaired.
    cli-utils.sigs.k8s.io/inventory-id: <INVENTORYID>
`

// InitOptions contains the fields necessary to generate a
// inventory object template ConfigMap.
type InitOptions struct {
	factory cmdutil.Factory

	ioStreams genericclioptions.IOStreams
	// Package directory argument; must be valid directory.
	Dir string
	// Namespace for inventory object; can not be empty.
	Namespace string
	// Inventory object label value; must be a valid k8s label value.
	InventoryID string
}

func NewInitOptions(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *InitOptions {
	return &InitOptions{
		factory:   f,
		ioStreams: ioStreams,
	}
}

// Complete fills in the InitOptions fields.
// TODO(seans3): Look into changing this kubectl-inspired way of organizing
// the InitOptions (e.g. Complete and Run methods).
func (i *InitOptions) Complete(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("need one 'directory' arg; have %d", len(args))
	}
	dir, err := normalizeDir(args[0])
	if err != nil {
		return err
	}
	i.Dir = dir

	ns, err := findNamespace(i.factory.ToRawKubeConfigLoader(), i.Dir)
	if err != nil {
		return err
	}
	i.Namespace = ns

	// Set the default inventory label if one does not exist.
	if len(i.InventoryID) == 0 {
		inventoryID, err := i.defaultInventoryID()
		if err != nil {
			return err
		}
		i.InventoryID = inventoryID
	}
	if !validateInventoryID(i.InventoryID) {
		return fmt.Errorf("invalid group name: %s", i.InventoryID)
	}
	// Output the calculated namespace used for inventory object.
	fmt.Fprintf(i.ioStreams.Out, "namespace: %s is used for inventory object\n", i.Namespace)
	return nil
}

type namespaceLoader interface {
	Namespace() (string, bool, error)
}

// findNamespace looks up the namespace that should be used for the
// inventory template of the package. If the namespace is specified with
// the --namespace flag, it will be used no matter what. If not, this
// will look at all the resource, and if all belong in the same namespace,
// it will return that namespace. Otherwise, it will return the namespace
// set in the context.
func findNamespace(loader namespaceLoader, dir string) (string, error) {
	namespace, enforceNamespace, err := loader.Namespace()
	if err != nil {
		return "", err
	}
	if enforceNamespace {
		return namespace, nil
	}

	ns, allInSameNs, err := allInSameNamespace(dir)
	if err != nil {
		return "", err
	}
	if allInSameNs {
		return ns, nil
	}
	return namespace, nil
}

// normalizeDir returns full absolute directory path of the
// passed directory or an error. This function cleans up paths
// such as current directory (.), relative directories (..), or
// multiple separators.
//
func normalizeDir(dirPath string) (string, error) {
	if !isDirectory(dirPath) {
		return "", fmt.Errorf("invalid directory argument: %s", dirPath)
	}
	return filepath.Abs(dirPath)
}

// isDirectory returns true if the passed path is a directory;
// false otherwise.
func isDirectory(path string) bool {
	if d, err := os.Stat(path); err == nil {
		if d.IsDir() {
			return true
		}
	}
	return false
}

// allInSameNamespace goes through all resources in the package and
// checks the namespace for all of them. If they all have the namespace
// set and they all have the same value, this will return that namespace
// and the second return value will be true. Otherwise, it will not return
// a namespace and the second return value will be false.
func allInSameNamespace(packageDir string) (string, bool, error) {
	r := kio.LocalPackageReader{PackagePath: packageDir}
	nodes, err := r.Read()
	if err != nil {
		return "", false, err
	}
	var ns string
	for _, node := range nodes {
		rm, err := node.GetMeta()
		if err != nil {
			return "", false, err
		}
		if rm.Namespace == "" {
			return "", false, nil
		}
		if ns == "" {
			ns = rm.Namespace
		} else if rm.Namespace != ns {
			return "", false, nil
		}
	}
	if ns != "" {
		return ns, true, nil
	}
	return "", false, nil
}

// defaultInventoryID returns a UUID string as a default unique
// identifier for a inventory object label.
func (i *InitOptions) defaultInventoryID() (string, error) {
	u, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Must begin and end with an alphanumeric character ([a-z0-9A-Z])
// with dashes (-), underscores (_), dots (.), and alphanumerics
// between.
const inventoryIDRegexp = `^[a-zA-Z0-9][a-zA-Z0-9\-\_\.]+[a-zA-Z0-9]$`

// validateInventoryID returns true of the passed group name is a
// valid label value; false otherwise. The valid label values
// are [a-z0-9A-Z] "-", "_", and "." The inventoryID must not
// be empty, but it can not be more than 63 characters.
func validateInventoryID(inventoryID string) bool {
	if len(inventoryID) == 0 || len(inventoryID) > 63 {
		return false
	}
	re := regexp.MustCompile(inventoryIDRegexp)
	return re.MatchString(inventoryID)
}

// fileExists returns true if a file at path already exists;
// false otherwise.
func fileExists(path string) bool {
	f, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !f.IsDir()
}

// fillInValues returns a string of the inventory object template
// ConfigMap with values filled in (eg. namespace, inventoryID).
// TODO(seans3): Look into text/template package.
func (i *InitOptions) fillInValues() string {
	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05 MST")
	randomSuffix := common.RandomStr(now.UTC().UnixNano())
	manifestStr := configMapTemplate
	manifestStr = strings.ReplaceAll(manifestStr, "<DATETIME>", nowStr)
	manifestStr = strings.ReplaceAll(manifestStr, "<NAMESPACE>", i.Namespace)
	manifestStr = strings.ReplaceAll(manifestStr, "<RANDOMSUFFIX>", randomSuffix)
	manifestStr = strings.ReplaceAll(manifestStr, "<INVENTORYID>", i.InventoryID)
	return manifestStr
}

func (i *InitOptions) Run() error {
	manifestFilePath := filepath.Join(i.Dir, manifestFilename)
	if fileExists(manifestFilePath) {
		return fmt.Errorf("inventory object template file already exists: %s", manifestFilePath)
	}
	f, err := os.Create(manifestFilePath)
	if err != nil {
		return fmt.Errorf("unable to create inventory object template file: %s", err)
	}
	defer f.Close()
	_, err = f.WriteString(i.fillInValues())
	if err != nil {
		return fmt.Errorf("unable to write inventory object template file: %s", manifestFilePath)
	}
	fmt.Fprintf(i.ioStreams.Out, "Initialized: %s\n", manifestFilePath)
	return nil
}

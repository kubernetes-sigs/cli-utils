[kind]: https://github.com/kubernetes-sigs/kind

# Demo: Init Command

This demo shows how the kapply init command works.

First define a place to work:

<!-- @makeWorkplace @testE2EAgainstLatestRelease -->
```
DEMO_HOME=$(mktemp -d)
```

Alternatively, use

> ```
> DEMO_HOME=~/demo
> ```

## Establish the base

<!-- @createBase @testE2EAgainstLatestRelease -->
```
BASE=$DEMO_HOME/base
mkdir -p $BASE
OUTPUT=$DEMO_HOME/output
mkdir -p $OUTPUT
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

function expectedOutputLine() {
  if ! grep -q "$@" "$OUTPUT/status"; then
    echo -e "${RED}Error: output line not found${NC}"
    echo -e "${RED}Expected: $@${NC}"
    exit 1
  else
    echo -e "${GREEN}Success: output line found${NC}"
  fi
}
```

## Create the first "app"

Create the config yaml for three config maps: (cm-a, cm-b, cm-c).

<!-- @createFirstConfigMaps @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: test-namespace
EOF

cat <<EOF >$BASE/config-map-a.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  namespace: test-namespace
  labels:
    name: test-config-map-label
EOF

cat <<EOF >$BASE/config-map-b.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-b
  namespace: test-namespace
  labels:
    name: test-config-map-label
EOF

cat <<EOF >$BASE/config-map-c.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-c
  namespace: test-namespace
  labels:
    name: test-config-map-label
EOF
```

## Run end-to-end tests

The following requires installation of [kind].

Delete any existing kind cluster and create a new one. By default the name of the cluster is "kind".

<!-- @deleteAndCreateKindCluster @testE2EAgainstLatestRelease -->
```
kind delete cluster
kind create cluster
```

Use the kapply init command to generate the inventory template. This contains
the namespace and inventory id used by apply to create inventory objects. 
<!-- @createInventoryTemplate @testE2EAgainstLatestRelease-->
```
kapply init $BASE | tee $OUTPUT/status
expectedOutputLine "namespace: test-namespace is used for inventory object"
```

Add another ConfigMap (cm-d) which is in the default namespace. The init
command should calculate the namespace to be default, since not all
objects are in the test-namespace.

<!-- @updateApp @testE2EAgainstLatestRelease -->
```

cat <<EOF >$BASE/config-map-d.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-d
  labels:
    name: test-config-map-label
EOF

# Remove the initial inventory template.
rm -f $BASE/inventory-template.yaml

kapply init $BASE | tee $OUTPUT/status
expectedOutputLine "namespace: default is used for inventory object"
```

Remove the ConfigMap (cm-d) which is in the default namespace, and
add a cluster-scoped object. This cluster-scoped object should not
be used in the init namespace calculations, so we should calculate the
namespace as test-namespace.

<!-- @updateAppAgain @testE2EAgainstLatestRelease -->
```

# Remove the initial inventory template.
rm -f $BASE/inventory-template.yaml

# Remove the ConfigMap in the default namespace.
rm -f $BASE/config-map-d.yaml

# Add cluster-scoped resource--cluster-role
cat <<EOF >$BASE/cluster-role.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  # "namespace" omitted since ClusterRoles are not namespaced
  name: secret-reader
rules:
- apiGroups: [""]
  #
  # at the HTTP level, the name of the resource for accessing Secret
  # objects is "secrets"
  resources: ["secrets"]
  verbs: ["get", "watch", "list"]
EOF

kapply init $BASE | tee $OUTPUT/status
expectedOutputLine "namespace: test-namespace is used for inventory object"
```


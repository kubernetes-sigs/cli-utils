[kind]: https://github.com/kubernetes-sigs/kind

# Demo: Inventory with Namespace

This demo shows that the namespace the inventory object
is applied into will get applied first, so the inventory
object will always have a namespace to be applied into.

First define a place to work:

<!-- @makeWorkplace @testE2EAgainstLatestRelease -->
```
DEMO_HOME=$(mktemp -d)
```

Alternatively, use

> ```
> DEMO_HOME=~/hello
> ```

## Establish the base

<!-- @createBase @testE2EAgainstLatestRelease -->
```
BASE=$DEMO_HOME/base
mkdir -p $BASE
OUTPUT=$DEMO_HOME/output
mkdir -p $OUTPUT

function expectedOutputLine() {
  test 1 == \
  $(grep "$@" $OUTPUT/status | wc -l); \
  echo $?
}
```

## Create the "app"

Create the config yaml for a config map and a namespace: (cm-a, test-namespace).

<!-- @createFirstConfigMaps @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/config-map-a.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  namespace: test-namespace
  labels:
    name: test-config-map-label
EOF

cat <<EOF >$BASE/test-namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: test-namespace
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
kapply init --namespace=test-namespace $BASE > $OUTPUT/status
expectedOutputLine "namespace: test-namespace is used for inventory object"
```

Apply the "app" to the cluster. The test-namespace should be configured, and
the config map should be created, and no resources should be pruned. The
test-namespace is created first, so the following resources within the namespace
(including the inventory object) will not fail.
<!-- @runApply @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "namespace/test-namespace unchanged"
expectedOutputLine "configmap/cm-a created"
expectedOutputLine "2 resource(s) applied. 1 created, 1 unchanged, 0 configured"

# There should be only one inventory object
kubectl get cm -n test-namespace --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"

# Capture the inventory object name for later testing
invName=$(kubectl get cm -n test-namespace --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | awk '{print $1}')

# There should be one config map that is not the inventory object
kubectl get cm -n test-namespace --selector='name=test-config-map-label' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"

# ConfigMap cm-a had been created in the cluster
kubectl get configmap/cm-a -n test-namespace --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
```

Now delete the inventory namespace from the local config. Ensure
that the subsequent apply does not prune this omitted namespace.
<!-- @noPruneInventoryNamespace @testE2EAgainstLatestRelease -->
```
rm -f $BASE/test-namespace.yaml
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "0 resource(s) pruned, 1 skipped"

# Inventory namespace should still exist
kubectl get ns test-namespace --no-headers | wc -l > $OUTPUT/status

# Inventory object should still exist
kubectl get cm/${invName} -n test-namespace --no-headers | wc -l > $OUTPUT/status

# ConfigMap cm-a should still exist
kubectl get configmap/cm-a -n test-namespace --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"

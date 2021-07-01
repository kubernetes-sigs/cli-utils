[kind]: https://github.com/kubernetes-sigs/kind

# Demo: Basic Prune

This demo shows basic pruning behavior by creating an
"app" with three config maps. After the initial apply of the
"app", pruning is demonstrated by locally deleting one
of the config maps, and applying again.

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

## Create the first "app"

Create the config yaml for three config maps: (cm-a, cm-b, cm-c).

<!-- @createFirstConfigMaps @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/config-map-a.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  labels:
    name: test-config-map-label
EOF

cat <<EOF >$BASE/config-map-b.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-b
  labels:
    name: test-config-map-label
EOF

cat <<EOF >$BASE/config-map-c.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-c
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
kapply init $BASE > $OUTPUT/status
expectedOutputLine "namespace: default is used for inventory object"
```

Apply the "app" to the cluster. All the config maps should be created, and
no resources should be pruned.
<!-- @runApply @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "configmap/cm-a created"
expectedOutputLine "configmap/cm-b created"
expectedOutputLine "configmap/cm-c created"

# There should be only one inventory object
kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# Capture the inventory object name for later testing
invName=$(kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | awk '{print $1}')
# There should be three config maps
kubectl get cm --selector='name=test-config-map-label' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "3"
# ConfigMap cm-a had been created in the cluster
kubectl get configmap/cm-a --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-b had been created in the cluster
kubectl get configmap/cm-b --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-c had been created in the cluster
kubectl get configmap/cm-c --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
```

## Update the "app" to remove a config map, and add another config map.

Remove cm-a.

Create a fourth config map--cm-d.
<!-- @createAnotherConfigMap @testE2EAgainstLatestRelease -->
```
rm -f $BASE/config-map-a.yaml

cat <<EOF >$BASE/config-map-d.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-d
  labels:
    name: test-config-map-label
EOF
```

## Apply the updated "app"

cm-a should be pruned (since it has been deleted locally).

cm-b, cm-c should be unchanged.

cm-d should be created.
<!-- @applySecondTime @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "configmap/cm-a pruned"
expectedOutputLine "configmap/cm-b unchanged"
expectedOutputLine "configmap/cm-c unchanged"
expectedOutputLine "configmap/cm-d created"
expectedOutputLine "1 resource(s) pruned, 0 skipped"

# There should be only one inventory object
kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# The inventory object should have the same name
kubectl get configmap/${invName} --no-headers > $OUTPUT/status
expectedOutputLine "${invName}"
# There should be three config maps
kubectl get cm --selector='name=test-config-map-label' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "3"
# ConfigMap cm-b had been created in the cluster
kubectl get configmap/cm-b --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-c had been created in the cluster
kubectl get configmap/cm-c --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-d had been created in the cluster
kubectl get configmap/cm-d --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
```

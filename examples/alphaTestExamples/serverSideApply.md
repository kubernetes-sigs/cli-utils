[kind]: https://github.com/kubernetes-sigs/kind

# Demo: Server Side Apply

This demo shows how to invoke server-side apply,
instead of the default client-side apply.

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

Create the config yaml for two config maps: (cm-a, cm-b).

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
data:
  foo: sean
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
<!-- @runServerSideApply @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --server-side --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "configmap/cm-a serversideapplied"
expectedOutputLine "configmap/cm-b serversideapplied"
expectedOutputLine "2 serverside applied"

# There should be only one inventory object
kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# There should be two config maps that are not the inventory object
kubectl get cm --selector='name=test-config-map-label' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "2"
# ConfigMap cm-a had been created in the cluster
kubectl get configmap/cm-a --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-b had been created in the cluster
kubectl get configmap/cm-b --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
```

Update a config map to update a field owned by the default field manager.
Update both config maps, using a different field-manager to create a
conflict, but the the --force-conflicts flag to overwrite successfully.
The conflicting field is "data.foo".
<!-- @runServerSideApplyWithForceConflicts @testE2EAgainstLatestRelease -->
```
cat <<EOF >$BASE/config-map-b.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-b
  labels:
    name: test-config-map-label
data:
  foo: baz
EOF

kapply apply $BASE --server-side --field-manager=sean --force-conflicts --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "configmap/cm-a serversideapplied"
expectedOutputLine "configmap/cm-b serversideapplied"
expectedOutputLine "2 serverside applied"
```

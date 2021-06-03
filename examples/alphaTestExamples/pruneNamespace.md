[kind]: https://github.com/kubernetes-sigs/kind

# Demo: Namespaces and Prune

This demo shows that namespaces will **not** be pruned if
there are objects remaining in them. The namespace for the
inventory object will be the default namespace, since not
all of the app objects are in the same namespace (pod-b
is in the default namespace). When pod-a, pod-b, and
the test-namespace are omitted from the subsequent apply
the test-namespace will be considered for pruning, but it
will **not** happend because there is still one object
in the namespace--pod-c.

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
kapply init $BASE > $OUTPUT/status
expectedOutputLine "namespace: default is used for inventory object"
```

Apply the "app" to the cluster. All the config maps should be created, and
no resources should be pruned.
<!-- @runApply @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "namespace/test-namespace created"
expectedOutputLine "configmap/cm-a created"
expectedOutputLine "configmap/cm-b created"
expectedOutputLine "configmap/cm-c created"
expectedOutputLine "4 resource(s) applied. 4 created, 0 unchanged, 0 configured"
expectedOutputLine "0 resource(s) pruned, 0 skipped"

# There should be only one inventory object
kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# Capture the inventory object name for later testing
invName=$(kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | awk '{print $1}')
# There should be four config maps: one inventory in default, two in test-namespace, one in default namespace
kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
kubectl get cm -n test-namespace --selector='name=test-config-map-label' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "2"
kubectl get cm --selector='name=test-config-map-label' --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-a had been created in the cluster
kubectl get configmap/cm-a -n test-namespace --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-b had been created in the cluster
kubectl get configmap/cm-b --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# ConfigMap cm-c had been created in the cluster
kubectl get configmap/cm-c -n test-namespace --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
```

## Update the "app" to remove a two of the config maps, and the
namespace.

Remove test-namespace
Remove cm-a
Remove cm-b

<!-- @createAnotherConfigMap @testE2EAgainstLatestRelease -->
```

rm -f $BASE/namespace.yaml
rm -f $BASE/config-map-a.yaml
rm -f $BASE/config-map-b.yaml

```

## Apply the updated "app"

cm-a should be pruned (since it has been deleted locally).
cm-b should be pruned (since it has been deleted locally).
test-namespace should **not** be pruned.

<!-- @applySecondTime @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
expectedOutputLine "configmap/cm-a pruned"
expectedOutputLine "configmap/cm-b pruned"
expectedOutputLine "configmap/cm-c unchanged"
expectedOutputLine "2 resource(s) pruned, 1 skipped"

# The test-namespace should not be pruned.
kubectl get ns test-namespace --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
# Inventory object should have two items: namespace and cm-c.
kubectl get cm --selector='cli-utils.sigs.k8s.io/inventory-id' --no-headers | awk '{print $2}'  > $OUTPUT/status
expectedOutputLine "2"
# The inventory object should have the same name
kubectl get configmap/${invName} --no-headers > $OUTPUT/status
expectedOutputLine "${invName}"
# ConfigMap cm-c remains in the cluster.
kubectl get configmap/cm-c -n test-namespace --no-headers | wc -l > $OUTPUT/status
expectedOutputLine "1"
```

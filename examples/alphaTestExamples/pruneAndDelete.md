[kind]: https://github.com/kubernetes-sigs/kind

# Demo: Lifecycle directives

This demo shows how it is possible to use a lifecycle directive to 
change the behavior of prune and delete for specific resources.

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

In this example we will just use two ConfigMap resources for simplicity, but
of course any type of resource can be used. On one of our ConfigMaps, we add the
**cli-utils.sigs.k8s.io/on-remove** annotation with the value of **keep**. This 
annotation tells the kapply tool that this resource should not be deleted, even
if it would otherwise be pruned or deleted with the destroy command.

<!-- @createFirstCM @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/configMap1.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: firstmap
data:
  artist: Ornette Coleman
  album: The shape of jazz to come
EOF
```

This ConfigMap includes the lifecycle directive annotation

<!-- @createSecondCM @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/configMap2.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: secondmap
  annotations:
    cli-utils.sigs.k8s.io/on-remove: keep
data:
  artist: Husker Du
  album: New Day Rising
EOF
```

## Run end-to-end tests

The following requires installation of [kind].

Delete any existing kind cluster and create a new one. By default the name of the cluster is "kind"
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

Apply both resources to the cluster.
<!-- @runApply @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
```

Use the preview command to show what will happen if we run destroy. This should
show that the second ConfigMap will not be deleted even when using the destroy 
command.
<!-- @runDestroyPreview @testE2EAgainstLatestRelease -->
```
kapply preview --destroy $BASE > $OUTPUT/status

expectedOutputLine "configmap/firstmap deleted (preview)"

expectedOutputLine "configmap/secondmap delete skipped (preview)"
```

We run the destroy command and see that the resource without the annotation
has been deleted, while the resource with the annotation is still in the 
cluster.
<!-- @runDestroy @testE2EAgainstLatestRelease -->
```
kapply destroy $BASE > $OUTPUT/status

expectedOutputLine "configmap/firstmap deleted"

expectedOutputLine "configmap/secondmap delete skipped"

expectedOutputLine "1 resource(s) deleted, 1 skipped"
expectedNotFound "resource(s) pruned"

kubectl get cm --no-headers | awk '{print $1}' > $OUTPUT/status
expectedOutputLine "secondmap"
```


Apply the resources back to the cluster so we can demonstrate the lifecycle
directive with pruning.
<!-- @runApplyAgain @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status
```

Delete the manifest for the second configmap
<!-- @runDeleteManifest @testE2EAgainstLatestRelease -->
```
rm $BASE/configMap2.yaml
```

Run preview to see that while secondmap would normally be pruned, it 
will instead be skipped due to the lifecycle directive.
<!-- @runPreviewForPrune @testE2EAgainstLatestRelease -->
```
kapply preview $BASE > $OUTPUT/status

expectedOutputLine "configmap/secondmap prune skipped (preview)"
```

Run apply and verify that secondmap is still in the cluster.
<!-- @runApplyToPrune @testE2EAgainstLatestRelease -->
```
kapply apply $BASE > $OUTPUT/status

expectedOutputLine "configmap/secondmap prune skipped"

kubectl get cm --no-headers | awk '{print $1}' > $OUTPUT/status
expectedOutputLine "secondmap"

kind delete cluster;
```

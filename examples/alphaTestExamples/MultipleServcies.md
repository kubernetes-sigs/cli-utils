[kind]: https://github.com/kubernetes-sigs/kind

# Demo: Multiple Services

The following demonstrates applying and destroying multiple services to a `kind` cluster.

Steps:
1. Download the resources files for wordpress, mysql services.
2. Spin-up kubernetes cluster on local using [kind].
3. Deploy the wordpress, mysql services using kapply and verify the status.
4. Destroy wordpress service and verify that only wordpress service is destroyed.

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

Download the example configs for services `mysql` and `wordpress`
<!-- @createBase @testE2EAgainstLatestRelease -->
```
BASE=$DEMO_HOME/base
mkdir -p $BASE
OUTPUT=$DEMO_HOME/output
mkdir -p $OUTPUT

mkdir $BASE/wordpress
mkdir $BASE/mysql

curl -s -o "$BASE/wordpress/#1.yaml" "https://raw.githubusercontent.com\
/kubernetes-sigs/kustomize\
/master/examples/wordpress/wordpress\
/{deployment,service}.yaml"

curl -s -o "$BASE/mysql/#1.yaml" "https://raw.githubusercontent.com\
/kubernetes-sigs/kustomize\
/master/examples/wordpress/mysql\
/{secret,deployment,service}.yaml"

function expectedOutputLine() {
  test 1 == \
  $(grep "$@" $OUTPUT/status | wc -l); \
  echo $?
}
```

Create a `grouping.yaml` resource. By this, you are defining the grouping of the current 
directories. kapply uses the unique label in this file to track any future state changes 
made to this directory. Make sure the label key is `cli-utils.sigs.k8s.io/inventory-id` 
and give any unique label value and DO NOT change it in future.

<!-- @createGroupingYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/mysql/grouping.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: inventory-map-mysql
  labels:
    cli-utils.sigs.k8s.io/inventory-id: mysql-app
EOF

cat <<EOF >$BASE/wordpress/grouping.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: inventory-map-wordpress
  labels:
    cli-utils.sigs.k8s.io/inventory-id: wordpress-app
EOF

```

Delete any existing kind cluster and create a new one. By default the name of the cluster is "kind"
<!-- @deleteAndCreateKindCluster @testE2EAgainstLatestRelease -->
```
kind delete cluster
kind create cluster
```

Let's apply the wordpress and mysql services.
<!-- @RunWordpressAndMysql @testE2EAgainstLatestRelease -->
```
kapply apply $BASE/mysql --wait-for-reconcile > $OUTPUT/status;

expectedOutputLine "deployment.apps/mysql is Current: Deployment is available. Replicas: 1"

expectedOutputLine "secret/mysql-pass is Current: Resource is always ready"

expectedOutputLine "configmap/inventory-map-mysql-57005c71 is Current: Resource is always ready"

expectedOutputLine "service/mysql is Current: Service is ready"

kapply apply $BASE/wordpress --wait-for-reconcile > $OUTPUT/status;

expectedOutputLine "configmap/inventory-map-wordpress-2fbd5b91 is Current: Resource is always ready"

expectedOutputLine "service/wordpress is Current: Service is ready"

expectedOutputLine "deployment.apps/wordpress is Current: Deployment is available. Replicas: 1"

```

Destroy one service and make sure that only that service is destroyed and clean-up the cluster.
<!-- @destroyAppDeleteKindCluster @testE2EAgainstLatestRelease -->
```
kapply destroy $BASE/wordpress > $OUTPUT/status;

expectedOutputLine "service/wordpress pruned"

expectedOutputLine "deployment.apps/wordpress pruned"

expectedOutputLine "configmap/inventory-map-wordpress-2fbd5b91 pruned"

test 3 == \
  $(grep "" $OUTPUT/status | wc -l); \
  echo $?

kind delete cluster;
```

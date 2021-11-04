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
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

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
  if ! grep -q "$@" "$OUTPUT/status"; then
    echo -e "${RED}Error: output line not found${NC}"
    echo -e "${RED}Expected: $@${NC}"
    exit 1
  else
    echo -e "${GREEN}Success: output line found${NC}"
  fi
}
```

Use the kapply init command to generate the inventory template. This contains
the namespace and inventory id used by apply to create inventory objects.
<!-- @createInventoryTemplate @testE2EAgainstLatestRelease-->
```
kapply init $BASE/mysql | tee $OUTPUT/status
expectedOutputLine "namespace: default is used for inventory object"

kapply init $BASE/wordpress | tee $OUTPUT/status
expectedOutputLine "namespace: default is used for inventory object"
```

Delete any existing kind cluster and create a new one. By default the name of the cluster is "kind"
<!-- @deleteAndCreateKindCluster @testE2EAgainstLatestRelease -->
```
kind delete cluster
kind create cluster
```

Let's apply the mysql service
<!-- @RunMysql @testE2EAgainstLatestRelease -->
```
kapply apply $BASE/mysql --reconcile-timeout=120s --status-events | tee $OUTPUT/status

expectedOutputLine "deployment.apps/mysql is Current: Deployment is available. Replicas: 1"

expectedOutputLine "secret/mysql-pass is Current: Resource is always ready"

expectedOutputLine "service/mysql is Current: Service is ready"

# Verify that we have the mysql resources in the cluster.
kubectl get all --no-headers --selector=app=mysql | wc -l | xargs | tee $OUTPUT/status
expectedOutputLine "4"

# Verify that we don't have any of the wordpress resources in the cluster. 
kubectl get all --no-headers --selector=app=wordpress | wc -l | xargs | tee $OUTPUT/status
expectedOutputLine "0"
```

And the apply the wordpress service
<!-- @RunWordpress @testE2EAgainstLatestRelease -->
```
kapply apply $BASE/wordpress --reconcile-timeout=120s --status-events | tee $OUTPUT/status

expectedOutputLine "service/wordpress is Current: Service is ready"

expectedOutputLine "deployment.apps/wordpress is Current: Deployment is available. Replicas: 1"

# Verify that we now have the wordpress resources in the cluster.
kubectl get all --no-headers --selector=app=wordpress | wc -l | xargs | tee $OUTPUT/status
expectedOutputLine "4"
```

Destroy one service and make sure that only that service is destroyed and clean-up the cluster.
<!-- @destroyAppDeleteKindCluster @testE2EAgainstLatestRelease -->
```
kapply destroy $BASE/wordpress | tee $OUTPUT/status;

expectedOutputLine "service/wordpress deleted"

expectedOutputLine "deployment.apps/wordpress deleted"

expectedOutputLine "2 resource(s) deleted, 0 skipped"

# Verify that we still have the mysql resources in the cluster.
kubectl get all --no-headers --selector=app=mysql | wc -l | xargs | tee $OUTPUT/status
expectedOutputLine "4"

# TODO: When we implement wait for prune/destroy, add a check here to make
# sure the wordpress resources are actually deleted.

kind delete cluster;
```

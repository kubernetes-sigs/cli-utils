[hello]: https://github.com/monopole/hello
[kind]: https://github.com/kubernetes-sigs/kind

# Demo: hello app

This demo helps you to deploy an example hello app end-to-end using `kapply`.

Steps:
1. Create the resources files.
2. Spin-up kubernetes cluster on local using [kind].
3. Deploy, modify and delete the app using `kapply` and verify the status.

First define a place to work:

<!-- @makeWorkplace @testE2EAgainstLatestRelease-->
```
DEMO_HOME=$(mktemp -d)
```

Alternatively, use

> ```
> DEMO_HOME=~/hello
> ```

## Establish the base

Let's run the [hello] service.

<!-- @createBase @testE2EAgainstLatestRelease-->
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

function expectedNotFound() {
  test 0 == \
  $(grep "$@" $OUTPUT/status | wc -l); \
  echo $?
}
```

Let's add a simple config map resource in `base`

<!-- @createConfigMapYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/configMap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: the-map1
  namespace: hellospace
data:
  altGreeting: "Good Morning!"
  enableRisky: "false"
EOF
```

Create a deployment file with the following example configuration

<!-- @createDeploymentYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: hello
  name: the-deployment
  namespace: hellospace
spec:
  replicas: 3
  selector:
    matchLabels:
      app: hello
  template:
    metadata:
      labels:
        app: hello
        deployment: hello
    spec:
      containers:
      - command:
        - /hello
        - --port=8080
        - --enableRiskyFeature=\$(ENABLE_RISKY)
        env:
        - name: ALT_GREETING
          valueFrom:
            configMapKeyRef:
              key: altGreeting
              name: the-map1
        - name: ENABLE_RISKY
          valueFrom:
            configMapKeyRef:
              key: enableRisky
              name: the-map1
        image: monopole/hello:1
        name: the-container
        ports:
        - containerPort: 8080
          protocol: TCP
EOF
```

Create `service.yaml` pointing to the deployment created above

<!-- @createServiceYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/service.yaml
kind: Service
apiVersion: v1
metadata:
  name: the-service
  namespace: hellospace
spec:
  selector:
    deployment: hello
  type: LoadBalancer
  ports:
  - protocol: TCP
    port: 8666
    targetPort: 8080
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

Create the `hellospace` namespace where we will install the resources.
<!-- @createNamespace @testE2EAgainstLatestRelease -->
```
kubectl create namespace hellospace
```

Use the kapply init command to generate the inventory template. This contains
the namespace and inventory id used by apply to create inventory objects. 
<!-- @createInventoryTemplate @testE2EAgainstLatestRelease-->
```
kapply init $BASE

ls -1 $BASE > $OUTPUT/status
expectedOutputLine "inventory-template.yaml"
```

Run preview to check which commands will be executed
<!-- @previewHelloApp @testE2EAgainstLatestRelease -->
```
kapply preview $BASE > $OUTPUT/status

expectedOutputLine "3 resource(s) applied. 3 created, 0 unchanged, 0 configured, 0 failed (preview)"

kapply preview $BASE --server-side > $OUTPUT/status

expectedOutputLine "3 resource(s) applied. 0 created, 0 unchanged, 0 configured, 0 failed, 3 serverside applied (preview-server)"

# Verify that preview didn't create any resources.
kubectl get all -n hellospace > $OUTPUT/status 2>&1
expectedOutputLine "No resources found in hellospace namespace."
```

Use the `kapply` binary in `MYGOBIN` to apply a deployment and verify it is successful.
<!-- @runHelloApp @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --reconcile-timeout=1m > $OUTPUT/status

expectedOutputLine "deployment.apps/the-deployment is Current: Deployment is available. Replicas: 3"

expectedOutputLine "service/the-service is Current: Service is ready"

expectedOutputLine "configmap/the-map1 is Current: Resource is always ready"

# Verify that we have the pods running in the cluster
kubectl get --no-headers pod -n hellospace | wc -l | xargs > $OUTPUT/status
expectedOutputLine "3"
```

Now let's replace the configMap with configMap2 apply the config, fetch and verify the status.
This should delete the-map1 from deployment and add the-map2.
<!-- @replaceConfigMapInHello @testE2EAgainstLatestRelease -->
```
cat <<EOF >$BASE/configMap2.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: the-map2
  namespace: hellospace
data:
  altGreeting: "Good Evening!"
  enableRisky: "false"
EOF

rm $BASE/configMap.yaml

kapply apply $BASE --reconcile-timeout=120s > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment is Current: Deployment is available. Replicas: 3"

expectedOutputLine "service/the-service is Current: Service is ready"

expectedOutputLine "configmap/the-map2 is Current: Resource is always ready"

expectedOutputLine "configmap/the-map1 pruned"

# Verify that the new configmap has been created and the old one pruned.
kubectl get cm -n hellospace --no-headers | awk '{print $1}' > $OUTPUT/status
expectedOutputLine "the-map2"
expectedNotFound "the-map1"
```

Clean-up the cluster 
<!-- @deleteKindCluster @testE2EAgainstLatestRelease -->
```
kapply preview $BASE --destroy > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment deleted (preview)"

expectedOutputLine "configmap/the-map2 deleted (preview)"

expectedOutputLine "service/the-service deleted (preview)"

kapply preview $BASE --destroy --server-side > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment deleted (preview-server)"

expectedOutputLine "configmap/the-map2 deleted (preview-server)"

expectedOutputLine "service/the-service deleted (preview-server)"

# Verify that preview all resources are still there after running preview.
kubectl get --no-headers all -n hellospace | wc -l | xargs > $OUTPUT/status
expectedOutputLine "6"

kapply destroy $BASE > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment deleted"

expectedOutputLine "configmap/the-map2 deleted"

expectedOutputLine "service/the-service deleted"

expectedOutputLine "3 resource(s) deleted, 0 skipped"
expectedNotFound "resource(s) pruned"

kind delete cluster;
```

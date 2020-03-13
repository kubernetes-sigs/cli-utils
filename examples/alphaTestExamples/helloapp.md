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
```

Let's add a simple config map resource in `base`

<!-- @createConfigMapYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/configMap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: the-map
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
              name: the-map
        - name: ENABLE_RISKY
          valueFrom:
            configMapKeyRef:
              key: enableRisky
              name: the-map
        image: monopole/hello:1
        name: the-container
        ports:
        - containerPort: 8080
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

Create a `grouping.yaml` resource. By this, you are defining the grouping of the current directory,
`base`. `kapply` uses the unique label in this file to track any future state changes made to this
directory. Make sure the label key is `cli-utils.sigs.k8s.io/inventory-id` and give any unique
label value and DO NOT change it in future.

<!-- @createGroupingYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/grouping.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: inventory-map
  namespace: hellospace
  labels:
    cli-utils.sigs.k8s.io/inventory-id: hello-app
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

Use the `kapply` binary in `MYGOBIN` to apply a deployment and verify it is successful.
<!-- @runHelloApp @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --wait-for-reconcile > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment is Current: Deployment is available. Replicas: 3"

expectedOutputLine "service/the-service is Current: Service is ready"

expectedOutputLine "configmap/inventory-map-cb5a8e is Current: Resource is always ready"

expectedOutputLine "configmap/the-map is Current: Resource is always ready"

```

Now let's replace the configMap with configMap2 apply the config, fetch and verify the status.
This should delete the-map from deployment and add the-map2.
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

kapply apply $BASE --wait-for-reconcile > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment is Current: Deployment is available. Replicas: 3"

expectedOutputLine "service/the-service is Current: Service is ready"

expectedOutputLine "configmap/inventory-map-db36ed56 is Current: Resource is always ready"

expectedOutputLine "configmap/the-map2 is Current: Resource is always ready"

expectedOutputLine "configmap/the-map pruned"

expectedOutputLine "configmap/inventory-map-cb5a8e pruned"

```

Clean-up the cluster 
<!-- @deleteKindCluster @testE2EAgainstLatestRelease -->
```
kapply preview $BASE --destroy > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment deleted (preview)"

expectedOutputLine "configmap/the-map2 deleted (preview)"

expectedOutputLine "service/the-service deleted (preview)"

expectedOutputLine "configmap/inventory-map-db36ed56 deleted (preview)"

kapply destroy $BASE > $OUTPUT/status;

expectedOutputLine "deployment.apps/the-deployment deleted"

expectedOutputLine "configmap/the-map2 deleted"

expectedOutputLine "service/the-service deleted"

expectedOutputLine "configmap/inventory-map-db36ed56 deleted"

kind delete cluster;
```

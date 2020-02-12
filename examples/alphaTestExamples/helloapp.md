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
```

Now lets add a simple config map resource to the `base`

<!-- @createConfigMapYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/configMap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: the-map
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

Create a `grouping.yaml` resource. By this, you are defining the grouping of the current directory, `base`.
`kapply` uses the unique label in this file to track any future state changes made to this directory.
Make sure the label key is `cli-utils.sigs.k8s.io/inventory-id` and give any unique label value and DO NOT change it in future.

<!-- @createGroupingYaml @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/grouping.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: inventory-map
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

Use the `kapply` binary in `MYGOBIN` to apply a deployment and verify it is successful.
<!-- @runHelloApp @testE2EAgainstLatestRelease -->
```
kapply apply -f $BASE --status > $OUTPUT/status;

echo $BASE

test 1 == \
  $(grep "deployment.apps/the-deployment is Current: Deployment is available. Replicas: 3" $OUTPUT/status | grep "" | wc -l); \
  echo $?

test 1 == \
  $(grep "service/the-service is Current: Service is ready" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/inventory-map-7eabe827 is Current: Resource is always ready" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/the-map is Current: Resource is always ready" $OUTPUT/status | wc -l); \
  echo $?

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
data:
  altGreeting: "Good Evening!"
  enableRisky: "false"
EOF

rm $BASE/configMap.yaml

kapply apply -f $BASE --status > $OUTPUT/status;

cat $OUTPUT/status;

test 1 == \
  $(grep "deployment.apps/the-deployment is Current: Deployment is available. Replicas: 3" $OUTPUT/status | grep "" | wc -l); \
  echo $?

test 1 == \
  $(grep "service/the-service is Current: Service is ready" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/inventory-map-f124085f is Current: Resource is always ready" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/the-map2 is Current: Resource is always ready" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/the-map pruned" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/inventory-map-7eabe827 pruned" $OUTPUT/status | wc -l); \
  echo $?


```

Clean-up the cluster 
<!-- @deleteKindCluster @testE2EAgainstLatestRelease -->
```
kapply destroy -f $BASE > $OUTPUT/status;

test 1 == \
  $(grep "deployment.apps/the-deployment pruned" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/the-map2 pruned" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "service/the-service pruned" $OUTPUT/status | wc -l); \
  echo $?

test 1 == \
  $(grep "configmap/inventory-map-f124085f pruned" $OUTPUT/status | wc -l); \
  echo $?

kind delete cluster;
```

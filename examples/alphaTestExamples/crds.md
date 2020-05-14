[kind]: https://github.com/kubernetes-sigs/kind

# Demo: CRDs

This demo shows how it is possible to apply both a CRD and a CR
using the CRD, in the same apply operation. This is not something
that is possible with kubectl.

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

Create the CRD and a CR.

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

CRD

<!-- @createCRD @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/crd.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: foos.custom.io
spec:
  group: custom.io
  names:
    kind: Foo
    plural: foos
  scope: Namespaced
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: A sample CRD
        properties:
          apiVersion:
            description: 'APIVersion'
            type: string
          kind:
            description: 'Kind'
            type: string
          metadata:
            type: object
          spec:
            description: The spec for the CRD
            properties:
              name:
                description: Name
                type: string
            required:
            - name
            type: object
        type: object
    served: true
    storage: true
    subresources: {}
EOF
```

CR

<!-- @createCR @testE2EAgainstLatestRelease-->
```
cat <<EOF >$BASE/cr.yaml
apiVersion: custom.io/v1alpha1
kind: Foo
metadata:
  name: example-foo
spec:
  name: abc
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

We will install this in the default namespace.

Use the kapply init command to generate the inventory template. This contains
the namespace and inventory id used by apply to create inventory objects. 
<!-- @createInventoryTemplate @testE2EAgainstLatestRelease-->
```
kapply init $BASE

ls -1 $BASE > $OUTPUT/status
expectedOutputLine "inventory-template.yaml"
```

Use the `kapply` binary in `MYGOBIN` to apply both the CRD and the CR.
<!-- @runApply @testE2EAgainstLatestRelease -->
```
kapply apply $BASE --wait-for-reconcile > $OUTPUT/status

expectedOutputLine "customresourcedefinition.apiextensions.k8s.io/foos.custom.io is Current: Resource is current"

expectedOutputLine "foo.custom.io/example-foo is Current: Resource is current"

kubectl get crd --no-headers | awk '{print $1}' > $OUTPUT/status
expectedOutputLine "foos.custom.io"

kubectl get foos.custom.io --no-headers | awk '{print $1}' > $OUTPUT/status
expectedOutputLine "example-foo"
```

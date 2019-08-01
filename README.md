# cli-utils

This repo will contains binaries that built from libraries in cli-runtime.

## commands
This repo can build a binary `cli-utils` by
```
GO111MODULE=on go generate
GO111MODULE=on go build 
```
Note that the binary name has not been finally decided yet and is subject to change. Currently there are following candidates:
```
k2
kapply
kctl
kutils
kcli
kut
(other better name)
```

It provides following builtin commands as well as [dynamic commands](docs/dy/sample/REAME.md).

```
Usage:
  cli-utils [command]

Available Commands:
  apply       Apply resource configurations.
  delete      Delete resources from a Kubernetes cluster.
  help        Help about any command
  prune       Prune obsolete resources.
```

`apply` has a `status` subcommand, which can display status for resources.

All of those commands can be run on a kustomization directory or a directory with raw Kubernetes yaml file.

```shell
# apply a kustomization directory to cluster
./cli-utils --kubeconfig ~/.kube/config apply cmd/test-manifests/hello

# display the status
./cli-utils --kubeconfig ~/.kube/config apply status cmd/test-manifests/hello --wait --every 2 --count 30

# delete the applied resources
./cli-utils --kubeconfig ~/.kube/config delete cmd/test-manifests/hello

# apply with watching for status
./cli-utils --kubeconfig ~/.kube/config apply status cmd/test-manifests/hello
```

## libraries
You can also use cli-utils as a library. It provides a library to run `apply`,
`status`, `prune` and `delete` on a list of Unstructured resources.

 ```Go
import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg"
	)
  c := pkg.InitializeCmd(io.Stdout, nil)
  var resources []unstructured.Unstructured
  err := c.Apply(resources)
  err = c.Status(resources)
  err = c.Prune(resources)
  err = c.Delete(resources)
```

 ## Examples
TODO: add examples

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-cli)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-cli)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

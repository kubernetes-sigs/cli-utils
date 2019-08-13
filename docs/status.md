# status

```
cli-utils apply status <dir>
```
works for both directories with or without `kustomization.yaml`.

## kustomization directory

First, apply resources from a kustomization directory.

```
cli-utils apply config/hello
```

Then, get the status of the applied resources
```
cli-utils apply status config/hello
```

The output is like the following
```
Pod/myapp-pod   Ready
ConfigMap/example-cfgmap   Ready
StatefulSet/web   Pending Replicas: 4/10
Deployment/frontend   Ready
```

Delete the applied resources by
```
cli-utils delete config/hello
```

## non kustomization directory

The [manifests](../config/manifests) example doesn't contain a `kustomization.yaml`, you can apply it by
```
cli-utils apply config/manifests
```

Then, get the status of the applied resources
```
cli-utils apply status config/manifests
```

The output is like the following
```
ConfigMap/example-cfgmap   Ready
Deployment/frontend   Ready
Pod/myapp-pod   Read
```

Delete the applied resources by
```
cli-utils delete config/manifests
```


# Kubernetes (K8s) ConfigMap support

This package allows you to use [ConfigMap](http://kubernetes.io/docs/user-guide/configmap/) objects in Kubernetes to 
drive the update of [dynamic](../README.md#dynamic-json-flag-with-a-validator-and-notifier) `dflag` at runtime of your service.

## Semantics

The `Updater` is split into two phases:
 
 * `Initialize()` - used on server startup which allows both `static` and `dynamic` flags to be updated from values
    stored in a `ConfigMap` 
 * `Start()` - kicking off a an [`fsnotify`](https://github.com/fsnotify/fsnotify) Go-routine which watches for updates 
   of values in the ConfigMap. To avoid races, this allows only to update `dynamic` flags.
   
## Code example

```go
// First parse the flags from the command line, as normal.
flag.Parse()
// Setup watcher and start watching for change (including initial read)
u, err := configmaps.Setup(flag.CommandLine, "/etc/dflag", logger)
if err != nil {
  logger.Fatalf("failed setting up: %v", err)
}
```

## In a nutshell

You define a ConfigMap with values for your flags.

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  creationTimestamp: 2016-09-09T09:14:38Z
  name: example-config
  namespace: default
data:
  example_my_dynamic_string: something
  example_my_dynamic_int: 20
  example_my_dynamic_json: |-
    {
      "policy": "allow",
      "rate": 50
    }
```

Then you just push it to your Kubernetes cluster:

```
# kubectl replace -f example.yaml
```

And all your jobs referencing this ConfigMap via a volume mount will see updates `dflag` updates to keys in your data. For an end to end example see [server_kube](../examples/server_kube).

## Caveats

 * Kubernetes `<= 1.3` validate ConfigMap keys against DNS names, meaning that certain common characters (e.g. `_`) are 
   not allowed. With `>=1.4`, ConfigMaps are validated against `[-._a-zA-Z0-9]+` RE2 regex.
 * With Kubernetes `<=1.4` ConfigMaps don't get updated async, but on pod changes and otherwise at least every 60s. See 
   [kubernetes#30189](https://github.com/kubernetes/kubernetes/issues/30189).
   



   

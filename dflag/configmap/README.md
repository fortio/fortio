# Kubernetes (K8s) ConfigMap support

This package allows you to use [ConfigMap](http://kubernetes.io/docs/user-guide/configmap/) objects in Kubernetes to 
drive the update of [dynamic](../README.md#dynamic-json-flag-with-a-validator-and-notifier) `dflag` at runtime of your service.

## Semantics

The `Updater` is optionally split into two phases:
 
 * `Initialize()` - used on server startup which allows both `static` and `dynamic` flags to be updated from values
    stored in a `ConfigMap` 
 * `Start()` - kicking off a an [`fsnotify`](https://github.com/fsnotify/fsnotify) Go-routine which watches for updates 
   of values in the ConfigMap. To avoid races, this allows only to update `dynamic` flags.
   
Or you can do all at once `Setup()`
   
## Code example

```go
// First parse the flags from the command line, as normal.
flag.Parse()
// Setup watcher and start watching for change (including initial read)
u, err := configmap.Setup(flag.CommandLine, "/etc/dflag", logger)
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

Sample pod using a volume configmap:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: debug
spec:
  containers:
    - name: shell
      image: ubuntu:latest
      command: [ "/bin/bash" ]
      args: [ "-c", "while true; do date; sleep 60; done" ]
      volumeMounts:
        - name: config-volume
          mountPath: /etc/my-config
  volumes:
    - name: config-volume
      configMap:
        name: example-config
```

   

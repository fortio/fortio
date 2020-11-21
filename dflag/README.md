This came from https://github.com/ldemailly/go-flagz, a fork of the code originally on https://github.com/mwitkow/go-flagz and https://github.com/improbable-eng/go-flagz with initial changes to get the go modules to work, reduce boiler plate needed for configmap watcher, avoid panic when there is extra whitespace, make the watcher work with regular files and relative paths and switched to standard golang flags.
And now further changes, simplification, etc... as part of fortio.

Thanks to [@mwitkow](https://github.com/mwitkow) for having created this originally.

# Fortio Dynamic Flags (was Go FlagZ)

[![Apache 2.0 License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Dynamic, thread-safe `flag` variables that can be modified at runtime through files, URL endpoint,
or [Kubernetes](http://kubernetes.io) configmap changes.

For a similar project for JVM languages (Java, scala) see [java-flagz](https://github.com/mwitkow/java-flagz)
 
## This sounds crazy. Why?

File-based or command-line configuration can only be changed when a service restarts. Dynamic flags provide
flexibility in normal operations and emergencies. Two examples:
 
 * A new feature launches that you want to A/B test. You want to gradually enable it for a certain fraction of user
 requests (1%, 5%, 20%, 50%, 100%) without the need to restart servers.
 * Your service is getting overloaded and you want to disable certain costly features. You can't afford 
 restarting because you'd lose important capacity.
 
All of this can be done simultaneously across a whole shard of your services.

## Features

 * compatible with standard go `flag` package
 * dynamic `flag` that are thread-safe and efficient:
   - `DynBool`
   - `DynInt64`
   - `DynFloat64`
   - `DynString`
   - `DynDuration`
   - `DynStringSlice`
   - `DynStringSet`
   - `DynJSON` - a `flag` that takes an arbitrary JSON struct
 * `validator` functions for each `flag`, allows the user to provide checks for newly set values
 * `notifier` functions allow user code to be subscribed to `flag` changes
 * Kubernetes `ConfigMap` watcher, see [configmap/README.md](configmap/README.md).
 * a HandlerFunc `endpoint.ListFlags` that allows for easy inspection of the service's runtime configuration
 * a HandlerFunc `endpoint.SetFlag` that let's you update the flag values

Here's a teaser of the debug endpoint:

![Status Endpoint](https://user-images.githubusercontent.com/3664595/88000279-1d225480-cab2-11ea-82ca-68658ad16148.png)

## Examples

Declare a single `flag.FlagSet` in some public package (e.g. `common.SharedFlagSet`) that you'll use throughout your server or stick to `flag.CommandLine` default flagset for your binary.


### Dynamic JSON flag with a validator and notifier

```go
var (
  limitsConfigFlag = dflag.DynJSON(
    common.SharedFlagSet, 
    "rate_limiting_config", 
    &rateLimitConfig{ DefaultRate: 10, Policy: "allow"},
    "Config for service's rate limit",
  ).WithValidator(rateLimitConfigValidator).WithNotifier(onRateLimitChange)
)
```

This declares a JSON flag of type `rateLimitConfig` with a default value. Whenever the config changes (statically or dynamically) the `rateLimitConfigValidator` will be called. If it returns no errors, the flag will be updated and `onRateLimitChange` will be called with both old and new, allowing the rate-limit mechanism to re-tune.

## Dynamic feature flags

```go
var (
  featuresFlag = dflag.DynStringSlice(common.SharedFlagSet, "enabled_features", []string{"fast_index"}, "list of enabled feature markers")
)
...
func MyHandler(resp http.ResponseWriter, req *http.Request) {
   ...
   if existsInStringSlice("fast_index", featuresFlag.Get()) {
     doFastIndex(req)
   }
   ...
}
```

All access to `featuresFlag`, which is a `[]string` flag, is synchronised across go-routines using `atomic` pointer swaps. 

## Complete example

See a [http server](examples/server_kube) complete example.

# Status

This code is *production* quality. It's been running happily in production in its earlier incarnation at Improbable for years and now everywhere fortio runs.

### License

`dflag` (was `go-flagz`) is released under the Apache 2.0 license. See the [LICENSE](LICENSE) file for details.

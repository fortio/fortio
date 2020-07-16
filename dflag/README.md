# Go FlagZ 

[![CircleCI Build](https://circleci.com/gh/ldemailly/go-flagz.svg?style=shield)](https://circleci.com/gh/ldemailly/go-flagz)
[![Go Report Card](https://goreportcard.com/badge/github.com/ldemailly/go-flagz)](http://goreportcard.com/report/ldemailly/go-flagz)
[![GoDoc](http://img.shields.io/badge/GoDoc-Reference-blue.svg)](https://godoc.org/github.com/ldemailly/go-flagz)
[![SourceGraph](https://sourcegraph.com/github.com/ldemailly/go-flagz/-/badge.svg)](https://sourcegraph.com/github.com/ldemailly/go-flagz/?badge)
[![codecov](https://codecov.io/gh/ldemailly/go-flagz/branch/master/graph/badge.svg)](https://codecov.io/gh/ldemailly/go-flagz)
[![Apache 2.0 License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Dynamic, thread-safe `flag` variables that can be modified at runtime through [etcd](https://github.com/coreos/etcd)
or [Kubernetes](http://kubernetes.io).

For a similar project for JVM languages (Java, scala) see [java-flagz](https://github.com/mwitkow/java-flagz)

Code originally on https://github.com/mwitkow/go-flagz and https://github.com/improbable-eng/go-flagz
 
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
   - `DynInt64`
   - `DynFloat64`
   - `DynString`
   - `DynDuration`
   - `DynStringSlice`
   - `DynJSON` - a `flag` that takes an arbitrary JSON struct
   - `DynProto3` - a `flag` that takes a `proto3` struct in JSONpb or binary form
 * `validator` functions for each `flag`, allows the user to provide checks for newly set values
 * `notifier` functions allow user code to be subscribed to `flag` changes
 * Kubernetes `ConfigMap` watcher, see [configmap/README.md](configmap/README.md).
 * `etcd` based watcher that syncs values from a distributed Key-Value store into the program's memory
 * Prometheus metric for checksums of the current flag configuration
 * a `/debug/flagz` HandlerFunc endpoint that allows for easy inspection of the service's runtime configuration

Here's a teaser of the debug endpoint:

![Status Endpoint](https://raw.githubusercontent.com/ldemailly/go-flagz/screenshots/screenshot_endpoint.png)

## Examples

Declare a single `pflag.FlagSet` in some public package (e.g. `common.SharedFlagSet`) that you'll use throughout your server.


### Dynamic JSON flag with a validator and notifier

```go
var (
  limitsConfigFlag = flagz.DynJSON(
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
  featuresFlag = flagz.DynStringSlice(common.SharedFlagSet, "enabled_features", []string{"fast_index"}, "list of enabled feature markers")
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

## Watching for changes from etcd

```go
// First parse the flags from the command line, as normal.
common.SharedFlagSet.Parse(os.Args[1:])
w, err := watcher.New(common.SharedFlagSet, etcdClient, "/my_service/flagz", logger)
if err != nil {
  logger.Fatalf("failed setting up %v", err)
}
// Read flagz from etcd and update their values in common.SharedFlagSet
if err := w.Initialize(); err != nil {
	log.Fatalf("failed setting up %v", err)
}
// Start listening of dynamic flags from etcd.
w.Start()
```

The `watcher`'s go-routine will watch for `etcd` value changes and synchronise them with values in memory. In case a value fails parsing or the user-specified `validator`, the key in `etcd` will be atomically rolled back.

## More examples:

 * [simple http server](examples/server)
 * [printing CLI command](examples/simple)

# Status

This code is *production* quality. It's been running happily in production at Improbable for a few months.

### License

`go-flagz` is released under the Apache 2.0 license. See the [LICENSE](LICENSE) file for details.

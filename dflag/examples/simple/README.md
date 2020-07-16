# Simple CLI demo

This demonstrates how dynamic values are being updated in a simple CLI app.

## Quick set up:

Download [etcd](https://github.com/coreos/etcd/releases), extract and make it available on your `$PATH`.

Launch `etcd` server serving from a `default.data` in `/tmp`:

```sh
cd /tmp
etcd 
```

Set up a set of flags:

```sh
etcdctl mkdir /example/flagz
etcdctl set /example/flagz/example_my_static_int 9090
etcdctl set /example/flagz/example_my_dynamic_int foo
```

Play around:

```sh
./simple_cli &
etcdctl set /example/flagz/example_my_dynamic_string bar
etcdctl set /example/flagz/example_my_dynamic_int 7777
etcdctl set /example/flagz/example_my_dynamic_string bad_value
```

Profit.
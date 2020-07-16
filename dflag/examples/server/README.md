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
etcdctl put /example/flagz/staticint 9090
etcdctl put /example/flagz/dynstring foo
```

Play around by launching the server and visitng [http://localhost:8080](http://localhost:8080):

```sh
./simplesrv &
etcdctl put /example/flagz/example_my_dynamic_string "I'm santient"
etcdctl put /example/flagz/example_my_dynamic_int 12345
```

Marvel at the [flagz endpoint](http://localhost:8080/debug/flagz)).
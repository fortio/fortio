module github.com/ldemailly/go-flagz

replace github.com/coreos/bbolt => go.etcd.io/bbolt v1.3.5

require (
	github.com/coreos/etcd v3.3.22+incompatible
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/golang/protobuf v1.3.2
	github.com/mwitkow/go-etcd-harness v0.0.0-20160325212926-4dc1cb3e1ff9
	github.com/prometheus/client_golang v1.2.0
	github.com/prometheus/tsdb v0.7.1 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/ugorji/go v1.1.1 // indirect
	go.etcd.io/etcd/v3 v3.3.0-rc.0.0.20200710174459-07461ecc8c03 // indirect
	golang.org/x/net v0.0.0-20190813141303-74dc4d7220e7
)

go 1.14

module fortio.org/fortio

go 1.19 // As a library the current version of fortio works with 1.18 (first version with generics) but tests use 1.19 features

require (
	fortio.org/assert v1.2.1
	fortio.org/cli v1.5.2
	fortio.org/dflag v1.7.1
	fortio.org/log v1.12.2
	fortio.org/scli v1.14.2
	fortio.org/sets v1.0.4
	fortio.org/testscript v0.3.1
	fortio.org/version v1.0.4
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	golang.org/x/net v0.22.0
	google.golang.org/grpc v1.62.1
)

// Local dev of dependencies changes
//replace (
//	fortio.org/assert => ../assert
// 	fortio.org/cli => ../cli
// 	fortio.org/dflag => ../dflag
// 	fortio.org/log => ../log
// 	fortio.org/scli => ../scli
// 	fortio.org/version => ../version
//)

require (
	fortio.org/struct2env v0.4.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	golang.org/x/exp v0.0.0-20240318143956-a85f2c67cd81 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.19.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)

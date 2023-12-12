module fortio.org/fortio

go 1.19 // As a library the current version of fortio works with 1.18 (first version with generics) but tests use 1.19 features

require (
	fortio.org/assert v1.2.0
	fortio.org/cli v1.5.1
	fortio.org/dflag v1.7.0
	fortio.org/log v1.12.0
	fortio.org/scli v1.13.1
	fortio.org/sets v1.0.3
	fortio.org/testscript v0.3.1
	fortio.org/version v1.0.3
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.4.0
	golang.org/x/net v0.19.0
	google.golang.org/grpc v1.60.0
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
	golang.org/x/exp v0.0.0-20231206192017-f3f8817b8deb // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

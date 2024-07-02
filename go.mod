module fortio.org/fortio

// As a library the current version of fortio works with 1.18 (first version with generics) but tests use 1.19 features
// Note we will switch soon to 1.22 for the linters
go 1.21

toolchain go1.22.5 // force download as 1.22.5 isn't fully ready yet

// toolchain go1.22.3 // this shouldn't be necessary - see https://github.com/golang/go/issues/66175#issuecomment-2010343876

require (
	fortio.org/assert v1.2.1
	fortio.org/cli v1.7.0
	fortio.org/dflag v1.7.2
	fortio.org/log v1.14.0
	fortio.org/scli v1.15.1
	fortio.org/sets v1.1.1
	fortio.org/testscript v0.3.1
	fortio.org/version v1.0.4
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	golang.org/x/net v0.26.0
	google.golang.org/grpc v1.65.0
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
	fortio.org/struct2env v0.4.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	golang.org/x/crypto/x509roots/fallback v0.0.0-20240626151235-a6a393ffd658 // indirect
	golang.org/x/exp v0.0.0-20240613232115-7f521ea00fb8 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/tools v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240701130421-f6361c86f094 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

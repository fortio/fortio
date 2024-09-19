module fortio.org/fortio

// As a library the current version of fortio works with 1.18 (first version with generics) but tests use 1.19 features
// Note we will switch soon to 1.22 for the linters, using 1.21 is neeed when using toolchain below and because of grpc update
go 1.21

// When needed, ie to force download of July 2nd 2024 go security and bug fix release,
// as 1.22.5 docker images were not there yet and ditto for action/setup-go
// toolchain go1.22.5
// But see also https://github.com/golang/go/issues/66175#issuecomment-2010343876

require (
	fortio.org/assert v1.2.1
	fortio.org/cli v1.9.0
	fortio.org/dflag v1.7.2
	fortio.org/log v1.16.0
	fortio.org/safecast v1.0.0
	fortio.org/scli v1.15.2
	fortio.org/sets v1.2.0
	fortio.org/testscript v0.3.2
	fortio.org/version v1.0.4
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	golang.org/x/net v0.29.0
	google.golang.org/grpc v1.66.2
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
	github.com/kortschak/goroutine v1.1.2 // indirect
	golang.org/x/crypto/x509roots/fallback v0.0.0-20240904212608-c9da6b9a4008 // indirect
	golang.org/x/exp v0.0.0-20240904232852-e7e105dedf7e // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	golang.org/x/tools v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

module fortio.org/fortio

// As a library the current version of fortio works with 1.18 (first version with generics) but tests use 1.19 features
// And we're started to use the new features in 1.22 and 1.23
// (in part forced by grpc). we force 1.22.3 because 1.23.2 has pretty severe bug (macos) even though I think "1.23" with
// no patch level would be better for the go.mod file.
go 1.23.7

// When needed, ie to force download of July 2nd 2024 go security and bug fix release,
// as 1.22.5 docker images were not there yet and ditto for action/setup-go
// But see also https://github.com/golang/go/issues/66175#issuecomment-2010343876

require (
	fortio.org/assert v1.2.1
	fortio.org/cli v1.9.2
	fortio.org/dflag v1.7.3
	fortio.org/log v1.17.1
	fortio.org/safecast v1.0.0
	fortio.org/scli v1.15.3
	fortio.org/sets v1.2.0
	fortio.org/testscript v0.3.2
	fortio.org/version v1.0.4
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	golang.org/x/net v0.38.0
	google.golang.org/grpc v1.71.0
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
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/protobuf v1.36.4 // indirect
)

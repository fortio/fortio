module fortio.org/fortio

// As a library the current version of fortio works with 1.18 (first version with generics) but tests use 1.19 features
// And we're started to use the new features in 1.22 and 1.23
// (in part forced by grpc). we force 1.22.3 because 1.23.2 has pretty severe bug (macos) even though I think "1.23" with
// no patch level would be better for the go.mod file.
go 1.24.0

// When needed, ie to force download of July 2nd 2024 go security and bug fix release,
// as 1.22.5 docker images were not there yet and ditto for action/setup-go
// But see also https://github.com/golang/go/issues/66175#issuecomment-2010343876
// toolchain go1.23.8
// toolchain default // that doesn't work either

require (
	fortio.org/assert v1.2.1
	fortio.org/cli v1.12.3
	fortio.org/dflag v1.9.3
	fortio.org/duration v1.0.4
	fortio.org/log v1.18.3
	fortio.org/safecast v1.2.0
	fortio.org/scli v1.19.0
	fortio.org/sets v1.3.0
	fortio.org/testscript v0.3.2
	fortio.org/version v1.0.4
	github.com/golang/protobuf v1.5.4
	github.com/google/uuid v1.6.0
	github.com/jhump/protoreflect v1.18.0
	golang.org/x/net v0.49.0
	google.golang.org/grpc v1.78.0
	grol.io/grol v0.98.0
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
	fortio.org/struct2env v0.4.2 // indirect
	fortio.org/terminal v0.63.3 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/jbuchbinder/gopnm v0.0.0-20220507095634-e31f54490ce0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.1 // indirect
	github.com/kortschak/goroutine v1.1.3 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/crypto/x509roots/fallback v0.0.0-20250406160420-959f8f3db0fb // indirect
	golang.org/x/image v0.35.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/term v0.39.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

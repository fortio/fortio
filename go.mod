module fortio.org/fortio

go 1.18

require (
	fortio.org/assert v1.1.4
	fortio.org/cli v1.1.0
	fortio.org/dflag v1.5.2
	fortio.org/log v1.3.0
	fortio.org/scli v1.3.1
	fortio.org/version v1.0.2
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.3.0
	golang.org/x/net v0.9.0
	google.golang.org/grpc v1.54.0
)

// Local dev of dependencies changes
// replace (
// 	fortio.org/cli => ../cli
// 	fortio.org/dflag => ../dflag
// 	fortio.org/log => ../log
// 	fortio.org/scli => ../scli
// 	fortio.org/version => ../version
// )

require (
	fortio.org/sets v1.0.2 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	golang.org/x/exp v0.0.0-20230303215020-44a13b063f3e // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	google.golang.org/genproto v0.0.0-20230223222841-637eb2293923 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
)

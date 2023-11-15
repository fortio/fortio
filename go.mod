module fortio.org/fortio

go 1.18

require (
	fortio.org/assert v1.2.0
	fortio.org/cli v1.4.2
	fortio.org/dflag v1.7.0
	fortio.org/log v1.11.0
	fortio.org/scli v1.12.1
	fortio.org/sets v1.0.3
	fortio.org/testscript v0.3.1
	fortio.org/version v1.0.3
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.4.0
	golang.org/x/net v0.18.0
	google.golang.org/grpc v1.59.0
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
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	golang.org/x/exp v0.0.0-20231110203233-9a3e6036ecaa // indirect
	golang.org/x/sys v0.14.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

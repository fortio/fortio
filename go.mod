module fortio.org/fortio

go 1.18

require (
	fortio.org/assert v1.2.0
	fortio.org/cli v1.4.2
	fortio.org/dflag v1.6.0
	fortio.org/log v1.11.0
	fortio.org/scli v1.12.0
	fortio.org/sets v1.0.3
	fortio.org/testscript v0.3.1
	fortio.org/version v1.0.3
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.4.0
	golang.org/x/net v0.17.0
	google.golang.org/grpc v1.58.3
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
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231009173412-8bfb1ae86b6c // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

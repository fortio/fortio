module fortio.org/fortio

go 1.18

require (
	fortio.org/assert v1.2.0
	fortio.org/cli v1.2.0
	fortio.org/dflag v1.5.2
	fortio.org/log v1.9.0
	fortio.org/scli v1.9.0
	fortio.org/testscript v0.3.1
	fortio.org/version v1.0.2
	github.com/golang/protobuf v1.5.3
	github.com/google/uuid v1.3.0
	golang.org/x/net v0.12.0
	google.golang.org/grpc v1.56.2
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
	fortio.org/sets v1.0.3 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	golang.org/x/exp v0.0.0-20230713183714-613f0c0eb8a1 // indirect
	golang.org/x/sys v0.10.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	golang.org/x/tools v0.8.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

module fortio.org/fortio

go 1.18

require (
	fortio.org/assert v1.1.4
	fortio.org/cli v1.1.0
	fortio.org/dflag v1.4.2
	fortio.org/log v1.2.2
	fortio.org/scli v1.1.0
	fortio.org/version v1.0.2
	github.com/golang/protobuf v1.5.2
	github.com/google/uuid v1.3.0
	golang.org/x/net v0.7.0
	google.golang.org/grpc v1.53.0
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
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	golang.org/x/exp v0.0.0-20230213192124-5e25df0256eb // indirect
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	google.golang.org/genproto v0.0.0-20230216225411-c8e22ba71e44 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
)

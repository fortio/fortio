all: test_go

test_go: test_proto3.proto
	PATH="${GOPATH}/bin:${PATH}" protoc \
	  -I. \
		-I${GOPATH}/src \
		--go_out=plugins=grpc:. \
		*.proto



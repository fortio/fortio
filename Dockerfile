# Build the binaries in larger image
FROM istio/fortio.build:v6 as build
WORKDIR /go/src/istio.io
COPY . fortio
# Submodule handling
RUN make -C fortio submodule
# Putting spaces in linker replaced variables is hard but does work.
RUN echo "$(date +'%Y-%m-%d %H:%M') $(cd fortio; git rev-parse HEAD)" > /build-info.txt
# Sets up the static directory outside of the go source tree and
# the default data directory to a /var/lib/... volume
RUN go version
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags \
  "-s -X istio.io/fortio/ui.resourcesDir=/usr/local/lib/fortio -X main.defaultDataDir=/var/lib/istio/fortio \
  -X \"istio.io/fortio/version.buildInfo=$(cat /build-info.txt)\" \
  -X istio.io/fortio/version.tag=$(cd fortio; git describe --tags) \
  -X istio.io/fortio/version.gitstatus=$(cd fortio; git status --porcelain | wc -l)" \
  -o fortio.bin istio.io/fortio
# Check we still build with go 1.8 (and macos does not break)
RUN /usr/local/go/bin/go version
RUN CGO_ENABLED=0 GOOS=darwin /usr/local/go/bin/go build -a -ldflags -s -o fortio.go18.mac istio.io/fortio
# Just check it stays compiling on Windows (would need to set the rsrcDir too)
RUN CGO_ENABLED=0 GOOS=windows go build -a -o fortio.exe istio.io/fortio
# Minimal image with just the binary and certs
FROM scratch as release
# NOTE: the list of files here, if updated, must be changed in release/Dockerfile.in too
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /go/src/istio.io/fortio/ui/static /usr/local/lib/fortio/static
COPY --from=build /go/src/istio.io/fortio/ui/templates /usr/local/lib/fortio/templates
COPY --from=build /go/src/istio.io/fortio.bin /usr/local/bin/fortio
EXPOSE 8079
EXPOSE 8080
EXPOSE 8081
VOLUME /var/lib/istio/fortio
ENTRYPOINT ["/usr/local/bin/fortio"]
# start the server mode (grpc ping on 8079, http echo and UI on 8080, redirector on 8081) by default
CMD ["server"]

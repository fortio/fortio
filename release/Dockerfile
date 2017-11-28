# Build the binaries in larger image
FROM fortio/fortio.build:v4 as build
WORKDIR /go/src/istio.io
COPY . fortio
# Demonstrate moving the static directory outside of the go source tree:
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X istio.io/fortio/ui.resourcesDir=/usr/local/lib/fortio -s' -o fortio.bin istio.io/fortio
# What we save should match the ../Dockerfile (minus the certs)
WORKDIR /stage
COPY --from=build /go/src/istio.io/fortio.bin usr/local/bin/fortio
COPY --from=build /go/src/istio.io/fortio/ui/static usr/local/lib/fortio/static
COPY --from=build /go/src/istio.io/fortio/ui/templates usr/local/lib/fortio/templates
RUN mkdir -p var/lib/istio/fortio
# So fortio ran as whoever can write there:
RUN chmod 1777 var/lib/istio/fortio # /tmp like perms
RUN mkdir /tgz
RUN tar cvfz /tgz/fortio-$(/go/src/istio.io/fortio.bin -version).tgz *
FROM scratch
COPY --from=build /tgz/ /tgz/

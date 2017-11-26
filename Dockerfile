# Build the binaries in larger image
FROM fortio/fortio.build:v4 as build
WORKDIR /go/src/istio.io
COPY . fortio
# Demonstrate moving the static directory outside of the go source tree:
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X istio.io/fortio/ui.resourcesDir=/usr/local/lib/fortio -s' -o fortio.bin istio.io/fortio
# Minimal image with just the binary and certs
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /go/src/istio.io/fortio.bin /usr/local/bin/fortio
COPY --from=build /go/src/istio.io/fortio/ui/static /usr/local/lib/fortio/static
COPY --from=build /go/src/istio.io/fortio/ui/templates /usr/local/lib/fortio/templates
EXPOSE 8079
EXPOSE 8080
VOLUME /var/lib/istio/fortio
ENTRYPOINT ["/usr/local/bin/fortio"]
# start the server mode (grpc ping on 8079, http echo and UI on 8080) by default
CMD ["server"]

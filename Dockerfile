# Build the binaries in larger image
FROM docker.io/fortio/fortio.build:v38 as build
WORKDIR /go/src/fortio.org
COPY . fortio
# We moved a lot of the logic into the Makefile so it can be reused in brew
# but that also couples the 2, this expects to find binaries in the right place etc
RUN make -C fortio official-build-version BUILD_DIR=/build OFFICIAL_BIN=../fortio_go_latest.bin
# Check we still build with go 1.8 (and macos does not break)
RUN if [ x"$(dpkg --print-architecture)" = x"amd64" ]; then make -C fortio official-build BUILD_DIR=/build OFFICIAL_BIN=../fortio_go_latest.mac GOOS=darwin GO_BIN=/usr/local/go/bin/go; fi
# Windows release, for now assumes running from fortio directory (static resources in .)
RUN if [ x"$(dpkg --print-architecture)" = x"amd64" ]; then make -C fortio official-build BUILD_DIR=/build LIB_DIR=. OFFICIAL_BIN=../fortio.exe GOOS=windows; fi
# Minimal image with just the binary and certs
FROM scratch as release
# NOTE: the list of files here, if updated, must be changed in release/Dockerfile.in too
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# TODO: get rid of *.bak, *~ and other spurious non source files
#COPY --from=build /go/src/fortio.org/fortio/ui/static /usr/share/fortio/static
#COPY --from=build /go/src/fortio.org/fortio/ui/templates /usr/share/fortio/templates
COPY --from=build /go/src/fortio.org/fortio_go_latest.bin /usr/bin/fortio
EXPOSE 8078
EXPOSE 8079
EXPOSE 8080
EXPOSE 8081
# configmap (dynamic flags)
VOLUME /etc/fortio
# data files etc
VOLUME /var/lib/fortio
WORKDIR /var/lib/fortio
ENTRYPOINT ["/usr/bin/fortio"]
# start the server mode (grpc ping on 8079, http echo and UI on 8080, redirector on 8081) by default
CMD ["server", "-config", "/etc/fortio"]

# Build the binaries in larger image
FROM docker.io/fortio/fortio.build:v70@sha256:a1a0c03170d4be21a94d62b7e0b5dd966135e8888e8b2bf97eb8511eb6ee778e as build
WORKDIR /build
COPY --chown=build:build . fortio
ARG MODE=install
# We moved a lot of the logic into the Makefile so it can be reused in brew
# but that also couples the 2, this expects to find binaries in the right place etc
RUN make -C fortio official-build-version BUILD_DIR=/build MODE=${MODE}

# Minimal image with just the binary and certs
FROM scratch as release
# NOTE: the list of files here, if updated, must be changed in release/Dockerfile.in too
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# TODO: get rid of *.bak, *~ and other spurious non source files
#COPY --from=build /build/fortio/ui/static /usr/share/fortio/static
#COPY --from=build /build/fortio/ui/templates /usr/share/fortio/templates
COPY --from=build /build/result/fortio /usr/bin/fortio
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
CMD ["server", "-config-dir", "/etc/fortio"]

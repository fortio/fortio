# Build the binaries in larger image
FROM docker.io/fortio/fortio.build:v50@sha256:fe69c193d8ad40eb0d791984881f3678aead02660b8e3468c757f717892ada4c as build
WORKDIR /build
COPY . fortio
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
CMD ["server", "-config", "/etc/fortio"]
